package profiles

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

const (
	importLockName     = "profiles.import.lock"
	importStagePrefix  = "profiles.import-"
	importManifestName = "import-manifest.json"
	maxStoreBytes      = 32 << 20
	maxCacheFileBytes  = 256 << 20
	maxCacheCollection = 1 << 30
)

var (
	ErrProfileAlreadyInitialized = errors.New("profile data is already initialized")
	ErrProfileImportLocked       = errors.New("profile import is locked by another instance")
	ErrProfileImportStaged       = errors.New("unfinished profile import requires explicit cleanup")
	ErrProfileSourceChanged      = errors.New("profile source changed during import")
	ErrProfileTargetConflict     = errors.New("profile target already exists")
	ErrProfileNotInitialized     = errors.New("profile data is not initialized; choose import or empty library")
)

// SourceInfo is a safe summary of a possible legacy source. It deliberately
// contains no subscription URL, token, raw config, or credential.
type SourceInfo struct {
	Path             string
	Label            string
	Available        bool
	ProfileCount     int
	CacheCount       int
	Missing          []string
	PossibleTestData bool
	Error            string
}

// SetupStatus describes the canonical profile state without creating it.
type SetupStatus struct {
	Initialized       bool
	TargetExists      bool
	Error             string
	UnfinishedStaging []string
	LockPresent       bool
}

type sourceSnapshot struct {
	path       string
	storeBytes []byte
	caches     map[string][]byte
	store      *Store
}

type importManifest struct {
	Version      int               `json:"version"`
	Source       string            `json:"source"`
	ProfileCount int               `json:"profile_count"`
	CacheCount   int               `json:"cache_count"`
	StoreSHA256  string            `json:"store_sha256"`
	CacheSHA256  map[string]string `json:"cache_sha256"`
}

// InspectSource reads only the profile metadata and local cache files needed
// for a possible import. It never performs a network refresh.
func InspectSource(sourceDir string) (SourceInfo, error) {
	sourceDir, err := absoluteDirectory(sourceDir)
	info := SourceInfo{Path: sourceDir, Label: "用户选择"}
	if err != nil {
		info.Error = err.Error()
		return info, err
	}

	snapshot, err := readSourceSnapshot(sourceDir)
	if err != nil {
		info.Error = safeSourceError(err)
		if strings.Contains(err.Error(), "missing airports.json") {
			info.Missing = []string{"airports.json"}
		}
		return info, err
	}
	info.Available = true
	info.ProfileCount = len(snapshot.store.Airports)
	info.CacheCount, info.Missing = summarizeCaches(snapshot)
	info.PossibleTestData = looksLikeTestData(sourceDir)
	return info, nil
}

// DiscoverSourceCandidates returns only the current directory and executable
// directory. The caller may add one explicit user-selected directory; there
// is intentionally no disk/worktree scan.
func DiscoverSourceCandidates() []SourceInfo {
	paths := make([]struct {
		path  string
		label string
	}, 0, 2)
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, struct {
			path  string
			label string
		}{cwd, "当前启动目录"})
	}
	if exe, err := os.Executable(); err == nil {
		if dir := filepath.Dir(exe); dir != "" {
			paths = append(paths, struct {
				path  string
				label string
			}{dir, "可执行文件目录"})
		}
	}

	result := make([]SourceInfo, 0, len(paths))
	seen := make(map[string]struct{})
	for _, candidate := range paths {
		abs, err := absoluteDirectory(candidate.path)
		if err != nil {
			continue
		}
		key := abs
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		info, inspectErr := InspectSource(abs)
		info.Label = candidate.label
		if inspectErr != nil {
			// A missing airports.json is a normal unavailable candidate. Keep
			// it visible so the user can understand why it was not selectable.
			info.Available = false
		}
		result = append(result, info)
	}
	return result
}

// InspectSetup reports the canonical state and any unfinished staging without
// loading a real profile into memory or creating an empty canonical store.
func (p Paths) InspectSetup() SetupStatus {
	status := SetupStatus{
		UnfinishedStaging: p.UnfinishedStaging(),
		LockPresent:       p.importLockPresent(),
	}
	if p.Dir == "" {
		status.Error = "profile data directory is not configured"
		return status
	}

	profileInfo, err := os.Stat(p.Dir)
	if err == nil {
		status.TargetExists = true
		if !profileInfo.IsDir() {
			status.Error = "profile data target is not a directory"
			return status
		}
	} else if !os.IsNotExist(err) {
		status.Error = "inspect profile data target failed"
		return status
	}

	storeInfo, err := os.Stat(p.StoreFile())
	if err != nil {
		if os.IsNotExist(err) {
			return status
		}
		status.Error = "inspect profile store failed"
		return status
	}
	if storeInfo.IsDir() {
		status.Error = "profile store is not a file"
		return status
	}
	if _, err := LoadStore(p.StoreFile()); err != nil {
		status.Error = "profile store is damaged: " + safeSourceError(err)
		return status
	}
	status.Initialized = true
	return status
}

// InitializeEmpty creates an explicit empty canonical profile world. It uses
// the same non-overwriting staging/publish path as a source import.
func (p Paths) InitializeEmpty() error {
	root, err := p.prepareImportRoot()
	if err != nil {
		return err
	}
	unlock, err := acquireImportLock(root)
	if err != nil {
		return err
	}
	defer unlock()

	if err := p.ensureTargetAbsent(); err != nil {
		return err
	}
	if len(p.UnfinishedStaging()) > 0 {
		return ErrProfileImportStaged
	}

	stage, err := createImportStage(root)
	if err != nil {
		return err
	}
	if err := writeStageFiles(stage, []byte("{\n  \"airports\": []\n}\n"), map[string][]byte{}); err != nil {
		return err
	}
	manifest := importManifest{
		Version:      1,
		Source:       "empty",
		ProfileCount: 0,
		CacheCount:   0,
		StoreSHA256:  digestHex([]byte("{\n  \"airports\": []\n}\n")),
		CacheSHA256:  map[string]string{},
	}
	if err := writeManifest(stage, manifest); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(stage, importManifestName)); err != nil {
		return fmt.Errorf("finalize empty profile staging: %w", err)
	}
	return os.Rename(stage, p.Dir)
}

// ImportFrom performs a confirmation-authorized, local-only profile/cache
// import. The caller must have obtained an explicit user choice first.
func (p Paths) ImportFrom(ctx context.Context, sourceDir string) error {
	return p.importWithOptions(ctx, sourceDir, nil)
}

// importWithOptions is kept internal so tests can exercise the pre-commit
// interruption boundary without exposing a production hook or file API.
func (p Paths) importWithOptions(ctx context.Context, sourceDir string, beforeCommit func()) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	sourceDir, err := absoluteDirectory(sourceDir)
	if err != nil {
		return err
	}
	if err := p.validateTargetPath(); err != nil {
		return err
	}
	if samePath(sourceDir, p.Dir) {
		return fmt.Errorf("profile source cannot be the canonical target")
	}

	snapshot, err := readSourceSnapshot(sourceDir)
	if err != nil {
		return err
	}
	root, err := p.prepareImportRoot()
	if err != nil {
		return err
	}
	unlock, err := acquireImportLock(root)
	if err != nil {
		return err
	}
	defer unlock()

	if err := p.ensureTargetAbsent(); err != nil {
		return err
	}
	if len(p.UnfinishedStaging()) > 0 {
		return ErrProfileImportStaged
	}

	stage, err := createImportStage(root)
	if err != nil {
		return err
	}
	if err := writeStageFiles(stage, snapshot.storeBytes, snapshot.caches); err != nil {
		return err
	}
	cacheHashes := make(map[string]string, len(snapshot.caches))
	for name, body := range snapshot.caches {
		cacheHashes[name] = digestHex(body)
	}
	if err := writeManifest(stage, importManifest{
		Version:      1,
		Source:       sourceDir,
		ProfileCount: len(snapshot.store.Airports),
		CacheCount:   len(snapshot.caches),
		StoreSHA256:  digestHex(snapshot.storeBytes),
		CacheSHA256:  cacheHashes,
	}); err != nil {
		return err
	}

	// Read the staged files back before publishing. This verifies that the
	// bytes and IDs which will become canonical are the source snapshot.
	staged, err := readSourceSnapshot(stage)
	if err != nil {
		return fmt.Errorf("verify profile staging: %w", err)
	}
	if !snapshotsEqual(snapshot, staged) {
		return fmt.Errorf("verify profile staging: bytes changed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if beforeCommit != nil {
		beforeCommit()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// A second complete read protects against a legacy process changing the
	// source while the stage was being written. The source is never deleted or
	// rewritten by this operation.
	latest, err := readSourceSnapshot(sourceDir)
	if err != nil {
		return fmt.Errorf("recheck profile source: %w", err)
	}
	if !snapshotsEqual(snapshot, latest) {
		return ErrProfileSourceChanged
	}

	if err := os.Remove(filepath.Join(stage, importManifestName)); err != nil {
		return fmt.Errorf("finalize profile staging: %w", err)
	}
	if err := p.ensureTargetAbsent(); err != nil {
		return err
	}
	if err := os.Rename(stage, p.Dir); err != nil {
		return fmt.Errorf("publish profile data: %w", err)
	}
	return nil
}

// DiscardUnfinishedImport is an explicit recovery action for a staging set
// left by a failed or interrupted import. It never touches canonical data.
func (p Paths) DiscardUnfinishedImport() error {
	if err := p.validateTargetPath(); err != nil {
		return err
	}
	root := filepath.Dir(p.Dir)
	for _, stage := range p.UnfinishedStaging() {
		if err := os.RemoveAll(filepath.Join(root, stage)); err != nil {
			return fmt.Errorf("discard unfinished profile import: %w", err)
		}
	}
	if err := os.Remove(filepath.Join(root, importLockName)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("discard profile import lock: %w", err)
	}
	return nil
}

func (p Paths) prepareImportRoot() (string, error) {
	if err := p.validateTargetPath(); err != nil {
		return "", err
	}
	root := filepath.Dir(p.Dir)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create profile data root: %w", err)
	}
	return root, nil
}

func (p Paths) validateTargetPath() error {
	if p.Dir == "" || !filepath.IsAbs(p.Dir) {
		return fmt.Errorf("profile data directory must be an absolute path")
	}
	return nil
}

func (p Paths) ensureTargetAbsent() error {
	if _, err := os.Lstat(p.Dir); err == nil {
		return ErrProfileTargetConflict
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect profile target: %w", err)
	}
	return nil
}

func (p Paths) importLockPresent() bool {
	if p.Dir == "" {
		return false
	}
	_, err := os.Lstat(filepath.Join(filepath.Dir(p.Dir), importLockName))
	return err == nil
}

// UnfinishedStaging returns stage directory names only; the manifest source
// path is intentionally not surfaced to the UI.
func (p Paths) UnfinishedStaging() []string {
	if p.Dir == "" {
		return nil
	}
	root := filepath.Dir(p.Dir)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	result := make([]string, 0)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), importStagePrefix) || !entry.IsDir() {
			continue
		}
		result = append(result, entry.Name())
	}
	sort.Strings(result)
	return result
}

func acquireImportLock(root string) (func(), error) {
	path := filepath.Join(root, importLockName)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil, ErrProfileImportLocked
		}
		return nil, fmt.Errorf("acquire profile import lock: %w", err)
	}
	if _, err := file.WriteString(fmt.Sprintf("pid=%d\n", os.Getpid())); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write profile import lock: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("flush profile import lock: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("close profile import lock: %w", err)
	}
	return func() { _ = os.Remove(path) }, nil
}

func createImportStage(root string) (string, error) {
	for attempt := 0; attempt < 3; attempt++ {
		var random [6]byte
		_, _ = rand.Read(random[:])
		name := fmt.Sprintf("%s%d-%s", importStagePrefix, time.Now().UnixNano(), hex.EncodeToString(random[:]))
		path := filepath.Join(root, name)
		if err := os.Mkdir(path, 0o700); err != nil {
			if os.IsExist(err) {
				continue
			}
			return "", fmt.Errorf("create profile staging: %w", err)
		}
		return path, nil
	}
	return "", fmt.Errorf("create profile staging: name collision")
}

func writeStageFiles(stage string, storeBytes []byte, caches map[string][]byte) error {
	if err := writeSyncedFile(filepath.Join(stage, StoreFileName), storeBytes, 0o600); err != nil {
		return fmt.Errorf("stage profile store: %w", err)
	}
	cacheDir := filepath.Join(stage, CacheDirName)
	if err := os.Mkdir(cacheDir, 0o700); err != nil {
		return fmt.Errorf("stage profile cache directory: %w", err)
	}
	names := make([]string, 0, len(caches))
	for name := range caches {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := writeSyncedFile(filepath.Join(cacheDir, name), caches[name], 0o600); err != nil {
			return fmt.Errorf("stage profile cache: %w", err)
		}
	}
	return nil
}

func writeManifest(stage string, manifest importManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeSyncedFile(filepath.Join(stage, importManifestName), data, 0o600)
}

func writeSyncedFile(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func readSourceSnapshot(sourceDir string) (sourceSnapshot, error) {
	storePath := filepath.Join(sourceDir, StoreFileName)
	storeBytes, err := readBoundedFile(storePath, maxStoreBytes)
	if err != nil {
		if os.IsNotExist(err) {
			return sourceSnapshot{}, fmt.Errorf("source is missing airports.json")
		}
		return sourceSnapshot{}, fmt.Errorf("read source profile store: %w", err)
	}
	var store Store
	if err := json.Unmarshal(storeBytes, &store); err != nil {
		return sourceSnapshot{}, fmt.Errorf("source profile store is damaged")
	}
	if err := validateStore(&store); err != nil {
		return sourceSnapshot{}, err
	}
	if store.Airports == nil {
		store.Airports = []*Airport{}
	}

	caches := make(map[string][]byte)
	cacheDir := filepath.Join(sourceDir, CacheDirName)
	cacheInfo, cacheErr := os.Lstat(cacheDir)
	if cacheErr == nil {
		if cacheInfo.Mode()&os.ModeSymlink != 0 || !cacheInfo.IsDir() {
			return sourceSnapshot{}, fmt.Errorf("source cache directory is unsafe")
		}
		entries, err := os.ReadDir(cacheDir)
		if err != nil {
			return sourceSnapshot{}, fmt.Errorf("read source cache directory: %w", err)
		}
		var total int64
		for _, entry := range entries {
			if strings.ContainsAny(entry.Name(), `/\\`) || entry.Name() == "." || entry.Name() == ".." {
				return sourceSnapshot{}, fmt.Errorf("source cache contains an unsafe file name")
			}
			path := filepath.Join(cacheDir, entry.Name())
			fileInfo, err := os.Lstat(path)
			if err != nil {
				return sourceSnapshot{}, fmt.Errorf("inspect source cache: %w", err)
			}
			if fileInfo.Mode()&os.ModeSymlink != 0 || fileInfo.IsDir() || !fileInfo.Mode().IsRegular() {
				return sourceSnapshot{}, fmt.Errorf("source cache contains an unsafe file")
			}
			if strings.ToLower(filepath.Ext(entry.Name())) != ".yaml" {
				continue
			}
			body, err := readBoundedFile(path, maxCacheFileBytes)
			if err != nil {
				return sourceSnapshot{}, fmt.Errorf("read source cache: %w", err)
			}
			if err := validateCache(body); err != nil {
				return sourceSnapshot{}, fmt.Errorf("source cache cannot be parsed")
			}
			total += int64(len(body))
			if total > maxCacheCollection {
				return sourceSnapshot{}, fmt.Errorf("source cache collection is too large")
			}
			caches[entry.Name()] = body
		}
	} else if !os.IsNotExist(cacheErr) {
		return sourceSnapshot{}, fmt.Errorf("inspect source cache directory: %w", cacheErr)
	}

	return sourceSnapshot{path: sourceDir, storeBytes: storeBytes, caches: caches, store: &store}, nil
}

func validateStore(store *Store) error {
	seen := make(map[string]struct{})
	for _, airport := range store.Airports {
		if airport == nil || strings.TrimSpace(airport.ID) == "" {
			return fmt.Errorf("source profile has an invalid profile id")
		}
		if filepath.IsAbs(airport.ID) || filepath.Base(airport.ID) != airport.ID || strings.ContainsAny(airport.ID, `/\\:`) || airport.ID == "." || airport.ID == ".." {
			return fmt.Errorf("source profile has an unsafe profile id")
		}
		if _, exists := seen[airport.ID]; exists {
			return fmt.Errorf("source profile has duplicate profile ids")
		}
		seen[airport.ID] = struct{}{}
	}
	return nil
}

func validateCache(body []byte) error {
	if len(bytes.TrimSpace(body)) == 0 {
		return fmt.Errorf("empty cache")
	}
	var doc map[interface{}]interface{}
	if err := yaml.Unmarshal(body, &doc); err != nil || doc == nil {
		return fmt.Errorf("invalid yaml")
	}
	return nil
}

func summarizeCaches(snapshot sourceSnapshot) (int, []string) {
	count := 0
	missing := make([]string, 0)
	for _, airport := range snapshot.store.Airports {
		if _, ok := snapshot.caches[airport.ID+".yaml"]; ok {
			count++
		} else {
			missing = append(missing, "缓存："+airport.ID)
		}
	}
	if len(snapshot.store.Airports) > 0 && len(snapshot.caches) == 0 {
		missing = append([]string{"airports-cache"}, missing...)
	}
	return count, missing
}

func snapshotsEqual(a, b sourceSnapshot) bool {
	if !bytes.Equal(a.storeBytes, b.storeBytes) || len(a.caches) != len(b.caches) {
		return false
	}
	for name, body := range a.caches {
		if !bytes.Equal(body, b.caches[name]) {
			return false
		}
	}
	return true
}

func readBoundedFile(path string, max int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("unsafe file")
	}
	if info.Size() > max {
		return nil, fmt.Errorf("file is too large")
	}
	return os.ReadFile(path)
}

func absoluteDirectory(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("source directory must be an absolute path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve source directory: %w", err)
	}
	if lstatInfo, lstatErr := os.Lstat(abs); lstatErr != nil {
		return "", fmt.Errorf("source directory is unavailable")
	} else if lstatInfo.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("source directory must not be a symbolic link")
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("source directory is unavailable")
	}
	if !info.IsDir() {
		return "", fmt.Errorf("source path is not a directory")
	}
	return filepath.Clean(abs), nil
}

func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func looksLikeTestData(path string) bool {
	lower := strings.ToLower(filepath.Clean(path))
	for _, marker := range []string{"worktree", "tmp", "temp", "fixture", "test"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func safeSourceError(err error) string {
	if err == nil {
		return ""
	}
	// Error strings from this package contain only generic validation text or
	// local paths; never append file contents or decoded subscription values.
	return err.Error()
}

func digestHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
