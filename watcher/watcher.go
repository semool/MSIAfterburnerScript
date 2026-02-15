package watcher

import (
	"log"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/mitchellh/go-ps"
	"golang.org/x/sys/windows"
)

// WinEvent constants for event-driven watching
const (
	eventSystemForeground = 0x0003
	eventObjectCreate     = 0x8000
	eventObjectDestroy    = 0x8001
	wndOutofcontext       = 0x0000
)

// Lazy-load necessary DLL procedures for performance.
var (
	user32                       = windows.NewLazySystemDLL("user32.dll")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procGetWindowTextLen         = user32.NewProc("GetWindowTextLengthW")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procSetWinEventHook          = user32.NewProc("SetWinEventHook")
	procUnhookWinEvent           = user32.NewProc("UnhookWinEvent")
	procGetMessageW              = user32.NewProc("GetMessageW")
	procTranslateMessage         = user32.NewProc("TranslateMessage")
	procDispatchMessageW         = user32.NewProc("DispatchMessageW")

	kernel32        = windows.NewLazySystemDLL("kernel32.dll")
	procOpenProcess = kernel32.NewProc("OpenProcess")
	procCloseHandle = kernel32.NewProc("CloseHandle")

	psapi                    = windows.NewLazySystemDLL("psapi.dll")
	procGetModuleFileNameExW = psapi.NewProc("GetModuleFileNameExW")

	callbackOnce sync.Once
	callbackPtr  uintptr
	globalWinEventProc uintptr
)

type enumContext struct {
	keywords []string
	found    string
}

// StartEventWatcher sets up Windows event hooks to listen for system events.
func StartEventWatcher(handler func()) {
	go func() {
		winEventProc := syscall.NewCallback(func(hWinEventHook syscall.Handle, event uint32, hwnd syscall.Handle, idObject int32, idChild int32, idEventThread uint32, dwmsEventTime uint32) uintptr {
			handler()
			return 0
		})

		hookForeground, _, err := procSetWinEventHook.Call(uintptr(eventSystemForeground), uintptr(eventSystemForeground), 0, winEventProc, 0, 0, uintptr(wndOutofcontext))
		if hookForeground == 0 {
			log.Fatalf("Fatal: Could not set foreground event hook: %v", err)
		}

		hookCreate, _, err := procSetWinEventHook.Call(uintptr(eventObjectCreate), uintptr(eventObjectDestroy), 0, winEventProc, 0, 0, uintptr(wndOutofcontext))
		if hookCreate == 0 {
			log.Fatalf("Fatal: Could not set create/destroy event hook: %v", err)
		}

		defer procUnhookWinEvent.Call(hookForeground)
		defer procUnhookWinEvent.Call(hookCreate)

		var msg struct{ Hwnd, Message, WParam, LParam, Time, Pt uintptr }
		for {
			ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(ret) == -1 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		}
	}()
}

// FirstActiveTarget checks for a target using partial matching, prioritizing the foreground application.
// It returns the *keyword* that was matched, and a boolean indicating if a match was found.
func FirstActiveTarget(targets map[string]string) (string, bool) {
	keywords := make([]string, 0, len(targets))
	for k := range targets {
		keywords = append(keywords, k)
	}

	if name, ok := getForegroundTarget(keywords); ok {
		return name, true
	}
	if name, ok := isProcessActive(keywords); ok {
		return name, true
	}
	if name, ok := isWindowActive(keywords); ok {
		return name, true
	}
	return "", false
}

// getForegroundTarget checks if the foreground app's process or title contains a keyword.
func getForegroundTarget(keywords []string) (string, bool) {
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return "", false
	}

	title := getWindowText(windows.HWND(hwnd))
	if title != "" {
		lowerTitle := strings.ToLower(title)
		for _, keyword := range keywords {
			if strings.Contains(lowerTitle, keyword) {
				return keyword, true
			}
		}
	}

	var pid uint32
	_, _, err := procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if err != nil {
		return "", false
	}
	if pid == 0 {
		return "", false
	}
	handle, _, _ := procOpenProcess.Call(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, 0, uintptr(pid))
	if handle == 0 {
		return "", false
	}
	defer func() {
		ret, _, err := procCloseHandle.Call(handle)
		if ret == 0 {
			log.Printf("Warning: Failed to close process handle %v: %v", handle, err)
		}
	}()

	buf := make([]uint16, windows.MAX_PATH)
	n, _, _ := procGetModuleFileNameExW.Call(handle, 0, uintptr(unsafe.Pointer(&buf[0])), windows.MAX_PATH)
	if n > 0 {
		exePath := windows.UTF16ToString(buf)
		lowerExeName := strings.ToLower(filepath.Base(exePath))
		for _, keyword := range keywords {
			if strings.Contains(lowerExeName, keyword) {
				return keyword, true
			}
		}
	}

	return "", false
}

// isProcessActive checks if any running process name contains a keyword.
func isProcessActive(keywords []string) (string, bool) {
	processes, err := ps.Processes()
	if err != nil {
		return "", false
	}
	for _, p := range processes {
		lowerExeName := strings.ToLower(p.Executable())
		for _, keyword := range keywords {
			if strings.Contains(lowerExeName, keyword) {
				return keyword, true
			}
		}
	}
	return "", false
}

// isWindowActive checks if any visible window title contains a keyword.
func getEnumWindowsCallback() uintptr {
	callbackOnce.Do(func() {
		callbackPtr = windows.NewCallback(func(hwnd windows.HWND, lParam uintptr) uintptr {
			ctx := (*enumContext)(unsafe.Pointer(lParam))
			visible, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
			if visible == 0 {
				return 1
			}
			title := getWindowText(hwnd)
			if title == "" {
				return 1
			}
			lower := strings.ToLower(title)
			for _, keyword := range ctx.keywords {
				if strings.Contains(lower, keyword) {
					ctx.found = keyword
					return 0 // stop enumeration
				}
			}
			return 1
		})
	})
	return callbackPtr
}

func isWindowActive(keywords []string) (string, bool) {
	ctx := enumContext{
		keywords: keywords,
		found:    "",
	}
	cb := getEnumWindowsCallback()
	ret, _, err := procEnumWindows.Call(
		cb,
		uintptr(unsafe.Pointer(&ctx)),
	)
	if ret == 0 && ctx.found == "" && err != nil {
		log.Printf("EnumWindows failed: %v", err)
	}
	if ctx.found != "" {
		return ctx.found, true
	}
	return "", false
}

func getWindowText(hwnd windows.HWND) string {
	length, _, _ := procGetWindowTextLen.Call(uintptr(hwnd))
	if length == 0 {
		return ""
	}
	buf := make([]uint16, length+1)
	ret, _, _ := procGetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buf[0])), length+1)
	if ret == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}
