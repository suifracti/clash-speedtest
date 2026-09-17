//go:build windows

package profiles

import "syscall"

func EnableUTF8Console() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setConsoleOutputCP := kernel32.NewProc("SetConsoleOutputCP")
	setConsoleCP := kernel32.NewProc("SetConsoleCP")
	const utf8 = 65001
	_, _, _ = setConsoleOutputCP.Call(uintptr(utf8))
	_, _, _ = setConsoleCP.Call(uintptr(utf8))
}
