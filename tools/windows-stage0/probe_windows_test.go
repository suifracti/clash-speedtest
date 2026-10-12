//go:build windows

package stage0

import (
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

// Test success means observations collected, not durability GO.
func TestWindowsStage0(t *testing.T) {
	log := func(name string, err error, f map[string]any) {
		t.Helper()
		f["probe"] = name
		f["api_success"] = err == nil
		var code syscall.Errno
		if errors.As(err, &code) {
			f["win32_error"] = uint32(code)
		}
		if err != nil {
			f["error"] = err.Error()
		}
		b, e := json.Marshal(f)
		if e != nil {
			t.Fatal(e)
		}
		t.Log(string(b))
	}
	ptr := func(s string) *uint16 {
		p, e := windows.UTF16PtrFromString(s)
		if e != nil {
			t.Fatal(e)
		}
		return p
	}
	root := t.TempDir() // All mutations derive ONLY from this synthetic subtree.
	token, e := windows.OpenCurrentProcessToken()
	if e != nil {
		t.Fatal(e)
	}
	defer token.Close()
	var elevation, size uint32
	if e = windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevation)), 4, &size); e != nil {
		t.Fatal(e)
	}
	user, e := token.GetTokenUser()
	if e != nil {
		t.Fatal(e)
	}
	admin, e := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if e != nil {
		t.Fatal(e)
	}
	member, e := windows.Token(0).IsMember(admin)
	if e != nil {
		t.Fatal(e)
	}
	v := windows.RtlGetVersion()
	log("platform", nil, map[string]any{"major": v.MajorVersion, "minor": v.MinorVersion, "build": v.BuildNumber, "go": runtime.Version(), "arch": runtime.GOARCH, "x_sys": "v0.47.0 (module pinned)", "token_elevation": elevation, "admin_member": member, "ordinary_token": elevation == 0 && !member})
	var fs [256]uint16
	var flags uint32
	e = windows.GetVolumeInformation(ptr(filepath.VolumeName(root)+`\`), nil, 0, nil, nil, &flags, &fs[0], 256)
	log("filesystem", e, map[string]any{"filesystem": windows.UTF16ToString(fs[:]), "flags": flags, "persistent_acls": flags&windows.FILE_PERSISTENT_ACLS != 0})
	if e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(root, "synthetic.txt")
	if e = os.WriteFile(file, []byte("synthetic stage0\n"), 0600); e != nil {
		t.Fatal(e)
	}
	for _, p := range []string{root, file} {
		info, e := os.Lstat(p)
		if e != nil {
			t.Fatal(e)
		}
		log("go_mode", nil, map[string]any{"directory": info.IsDir(), "mode": fmt.Sprintf("%04o", info.Mode().Perm())})
	}
	for _, a := range []uint32{windows.GENERIC_READ, windows.GENERIC_WRITE, windows.GENERIC_READ | windows.GENERIC_WRITE} {
		h, e := windows.CreateFile(ptr(file), a, 7, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		log("file_open", e, map[string]any{"access": a, "share": 7, "flags": windows.FILE_ATTRIBUTE_NORMAL})
		if e == nil {
			log("file_flush", windows.FlushFileBuffers(h), map[string]any{"access": a})
			log("file_close", windows.CloseHandle(h), map[string]any{})
		}
	}
	wt := filepath.Join(root, "write-through.txt")
	h, e := windows.CreateFile(ptr(wt), windows.GENERIC_WRITE, 7, nil, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_WRITE_THROUGH, 0)
	log("write_through_create", e, map[string]any{"access": windows.GENERIC_WRITE, "share": 7, "flags": windows.FILE_FLAG_WRITE_THROUGH})
	if e == nil {
		var n uint32
		log("write_through_write", windows.WriteFile(h, []byte("synthetic write through"), &n, nil), map[string]any{})
		log("write_through_flush", windows.FlushFileBuffers(h), map[string]any{})
		log("write_through_close", windows.CloseHandle(h), map[string]any{})
	}
	child := filepath.Join(root, "new-directory-entry")
	if e = os.Mkdir(child, 0700); e != nil {
		t.Fatal(e)
	}
	// Fresh child and its parent: access x sharing x write-through matrix.
	for _, p := range []struct{ label, path string }{{"child", child}, {"parent", root}} {
		for _, a := range []uint32{0, windows.GENERIC_READ, windows.GENERIC_WRITE, windows.GENERIC_READ | windows.GENERIC_WRITE} {
			for _, share := range []uint32{0, 3, 7} {
				for _, extra := range []uint32{0, windows.FILE_FLAG_WRITE_THROUGH} {
					flags := uint32(windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT) | extra
					h, e := windows.CreateFile(ptr(p.path), a, share, nil, windows.OPEN_EXISTING, flags, 0)
					f := map[string]any{"object": p.label, "access": a, "share": share, "flags": flags}
					log("directory_open", e, f)
					if e == nil {
						log("directory_flush", windows.FlushFileBuffers(h), map[string]any{"object": p.label, "access": a, "share": share, "flags": flags})
						log("directory_close", windows.CloseHandle(h), map[string]any{"object": p.label})
					}
				}
			}
		}
	}
	sid := user.User.Sid.String()
	sd, e := windows.SecurityDescriptorFromString("O:" + sid + "D:P(A;OICI;FA;;;" + sid + ")")
	if e != nil {
		t.Fatal(e)
	}
	sa := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	private := filepath.Join(root, "private-acl")
	e = windows.CreateDirectory(ptr(private), &sa)
	log("private_directory_create", e, map[string]any{})
	if e != nil {
		t.Fatal(e)
	}
	pf := filepath.Join(private, "inherited.txt")
	if e = os.WriteFile(pf, []byte("synthetic private"), 0600); e != nil {
		t.Fatal(e)
	}
	for _, p := range []struct{ label, path string }{{"private_root", private}, {"inherited_file", pf}} {
		h, e := windows.CreateFile(ptr(p.path), windows.READ_CONTROL, 7, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if e != nil {
			t.Fatal(e)
		}
		d, e := windows.GetSecurityInfo(h, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
		if e != nil {
			windows.CloseHandle(h)
			t.Fatal(e)
		}
		o, _, e := d.Owner()
		if e != nil {
			windows.CloseHandle(h)
			t.Fatal(e)
		}
		c, _, e := d.Control()
		if e != nil {
			windows.CloseHandle(h)
			t.Fatal(e)
		}
		acl, _, e := d.DACL()
		if e != nil {
			windows.CloseHandle(h)
			t.Fatal(e)
		}
		var info windows.ByHandleFileInformation
		e = windows.GetFileInformationByHandle(h, &info)
		log("acl_readback", e, map[string]any{"object": p.label, "owner_matches": o.Equals(user.User.Sid), "dacl_nonnull": acl != nil, "protected": c&windows.SE_DACL_PROTECTED != 0, "sddl": strings.ReplaceAll(d.String(), sid, "<current-user>"), "reparse": info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0, "links": info.NumberOfLinks})
		if e = windows.CloseHandle(h); e != nil {
			t.Fatal(e)
		}
	}
	link := filepath.Join(private, "published.txt")
	log("hardlink_publish", os.Link(pf, link), map[string]any{})
	log("hardlink_existing_target", os.Link(pf, link), map[string]any{"expected_failure": true})
	b, e := os.ReadFile(link)
	log("hardlink_read", e, map[string]any{"data_equal": string(b) == "synthetic private"})
	log("hardlink_remove_source_name", os.Remove(pf), map[string]any{})
	h, e = windows.CreateFile(ptr(link), windows.GENERIC_READ, 3, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if e != nil {
		t.Fatal(e)
	}
	log("unlink_no_delete_share", os.Remove(link), map[string]any{"expected_failure": true})
	log("sharing_handle_close", windows.CloseHandle(h), map[string]any{})
	log("unlink_after_close", os.Remove(link), map[string]any{})
	if e = os.WriteFile(filepath.Join(child, "entry.txt"), []byte("synthetic"), 0600); e != nil {
		t.Fatal(e)
	}
	log("remove_nonempty_directory", os.Remove(child), map[string]any{"expected_failure": true})
	dst := filepath.Join(root, "renamed.txt")
	log("move_write_through", windows.MoveFileEx(ptr(wt), ptr(dst), windows.MOVEFILE_WRITE_THROUGH), map[string]any{"same_volume": true, "flags": windows.MOVEFILE_WRITE_THROUGH})
	log("move_existing_destination", windows.MoveFileEx(ptr(file), ptr(dst), windows.MOVEFILE_WRITE_THROUGH), map[string]any{"expected_failure": true, "replace_existing": false})
	junction := filepath.Join(root, "junction")
	e = exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", junction, child).Run()
	log("junction_create", e, map[string]any{})
	if e == nil {
		h, e = windows.CreateFile(ptr(junction), 0, 7, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		log("junction_open_no_follow", e, map[string]any{})
		if e == nil {
			var i windows.ByHandleFileInformation
			e = windows.GetFileInformationByHandle(h, &i)
			log("junction_readback", e, map[string]any{"reparse": i.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0})
			windows.CloseHandle(h)
		}
		log("junction_remove_link", os.Remove(junction), map[string]any{})
	}
	log("stage0_verdict", nil, map[string]any{"verdict": "BLOCKED", "ordinary_token": elevation == 0 && !member, "directory_entry_durability_proven": false, "power_loss_tested": false, "reason": "API observations cannot establish documented new-directory-entry durability or power-loss guarantee; production compatibility unchanged"})
}
