//go:build darwin && cgo

package main

// Wails 2.16 native dialogs reference UTType. Declare its framework explicitly
// so both direct Go production builds and Wails builds link on current SDKs.
// #cgo LDFLAGS: -framework UniformTypeIdentifiers
import "C"
