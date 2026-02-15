//go:build windows
package trayicon

import (
	_ "embed"
	"syscall"
)

var (
	//go:embed icon.ico
	IconData []byte
	consoleVisible = true

	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")

	procGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")
	procShowWindow       = user32.NewProc("ShowWindow")
)

const (
	SW_HIDE = 0
	SW_SHOW = 5
)

func getConsoleWindow() syscall.Handle {
	hwnd, _, _ := procGetConsoleWindow.Call()
	return syscall.Handle(hwnd)
}

func HideConsole() {
	hwnd := getConsoleWindow()
	if hwnd != 0 {
		procShowWindow.Call(uintptr(hwnd), SW_HIDE)
		consoleVisible = false
	}
}

func showConsole() {
	hwnd := getConsoleWindow()
	if hwnd != 0 {
		procShowWindow.Call(uintptr(hwnd), SW_SHOW)
		consoleVisible = true
	}
}

func ToggleConsole() {
	if consoleVisible {
		HideConsole()
	} else {
		showConsole()
	}
}
