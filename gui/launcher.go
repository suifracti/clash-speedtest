package gui

import (
	"os/exec"
)

type BrowserInfo struct {
	ID        string `json:"id"`        // "chrome", "edge", "default", "custom"
	Name      string `json:"name"`      // Display name
	Path      string `json:"path"`      // Executable path if known
	Installed bool   `json:"installed"` // Whether it is detected on the machine
}

// DesktopLauncher handles launching the dedicated application window.
type DesktopLauncher interface {
	Launch(url string, preferred string) (*exec.Cmd, error)
}
