package logger

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"syscall"
	"sync"
	"runtime"
	"unsafe"

	"MSIAfterburnerProfileSwitcher/trayicon"
)

var (
	logBuffer     bytes.Buffer
	logMutex      sync.Mutex
	logWindowHwnd syscall.Handle
	logEditHwnd   syscall.Handle
	logWindowOpen bool
	hBackgroundBrush syscall.Handle
	consoleVisible = true

	globalLogger = struct {
		subscribers []chan string
		mutex       sync.Mutex
	}{}

	windowChannel chan string

	windowWide = 1024
	windowHeight = 768

	fontName = "Consolas"
	fontSize = -15

	gdi32 = syscall.NewLazyDLL("gdi32.dll")
	user32 = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
)

// Logger
type winLogWriter struct{}

func (w winLogWriter) Write(p []byte) (n int, err error) {
	logMutex.Lock()
	defer logMutex.Unlock()
	logBuffer.Write(p)
	globalLogger.mutex.Lock()
	defer globalLogger.mutex.Unlock()

	for _, ch := range globalLogger.subscribers {
		select {
		case ch <- string(p):
		default:
		}
	}
	return len(p), nil
}

// Start Logger
func InitLogger() {
	//log.SetFlags(log.Ltime)

	if consoleVisible {
		// Console exist
		log.SetOutput(io.MultiWriter(winLogWriter{}, os.Stdout))
	} else {
		// No Console -ldflags="-H windowsgui"
		log.SetOutput(winLogWriter{})
	}
}

// Append Text
func appendTextToEdit(text string) {
	if logEditHwnd == 0 {
		return
	}

	text = strings.ReplaceAll(text, "\n", "\r\n")
	EM_SETSEL := uintptr(0x00B1)
	EM_REPLACESEL := uintptr(0x00C2)
	user32.NewProc("SendMessageW").Call(uintptr(logEditHwnd), EM_SETSEL, ^uintptr(0), ^uintptr(0))
	user32.NewProc("SendMessageW").Call(uintptr(logEditHwnd), EM_REPLACESEL, 0, uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(text))))
}

//Set Window Icon from embedded Icon
func setWindowIconFromICO(hwnd syscall.Handle, iconData []byte) {
	if hwnd == 0 || len(iconData) == 0 {
		return
	}

	tmpFile, err := ioutil.TempFile("", "icon-*.ico")

	if err != nil {
		return
	}

	defer os.Remove(tmpFile.Name())
	tmpFile.Write(iconData)
	tmpFile.Close()
	IMAGE_ICON := uintptr(1)
	LR_LOADFROMFILE := uintptr(0x00000010)
	ICON_BIG := uintptr(1)
	ICON_SMALL := uintptr(0)

	hIcon, _, _ := user32.NewProc("LoadImageW").Call(
		0,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(tmpFile.Name()))),
		IMAGE_ICON,
		32, 32,
		LR_LOADFROMFILE,
	)

	if hIcon != 0 {
		const WM_SETICON = 0x80
		user32.NewProc("SendMessageW").Call(uintptr(hwnd), WM_SETICON, ICON_BIG, hIcon)
		user32.NewProc("SendMessageW").Call(uintptr(hwnd), WM_SETICON, ICON_SMALL, hIcon)
	}
}

// Open/Focus Log Window
func OpenOrFocusLogWindow() {
	if logWindowOpen && logWindowHwnd != 0 {
		user32.NewProc("ShowWindow").Call(uintptr(logWindowHwnd), 5)
		user32.NewProc("SetForegroundWindow").Call(uintptr(logWindowHwnd))
		return
	}
	go createLogWindow()
}

// Create Log Window
func createLogWindow() {
	runtime.LockOSThread()

	// black bg
	brush, _, _ := gdi32.NewProc("CreateSolidBrush").Call(0x000000)
	hBackgroundBrush = syscall.Handle(brush)

	className := syscall.StringToUTF16Ptr("MyLogWindowClass")

	const WM_CTLCOLOREDIT = 0x0133
	wndProc := syscall.NewCallback(func(hwnd syscall.Handle, msg uint32, wparam, lparam uintptr) uintptr {
	switch msg {

	case WM_CTLCOLOREDIT:
		hdc := wparam

		// white Text
		gdi32.NewProc("SetTextColor").Call(hdc, 0x00FFFFFF) // RGB(255,255,255)
		// black bg
		gdi32.NewProc("SetBkColor").Call(hdc, 0x00000000) // RGB(0,0,0)

		return uintptr(hBackgroundBrush)

	case 0x0002:
		logMutex.Lock()
		logWindowOpen = false
		logWindowHwnd = 0
		logEditHwnd = 0
		logMutex.Unlock()
		return 0
	}

	ret, _, _ := user32.NewProc("DefWindowProcW").Call(uintptr(hwnd), uintptr(msg), wparam, lparam)
	return ret
})

	hInstance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	var wc struct {
		Style         uint32
		LpfnWndProc   uintptr
		CbClsExtra    int32
		CbWndExtra    int32
		HInstance     uintptr
		HIcon         uintptr
		HCursor       uintptr
		HbrBackground uintptr
		LpszMenuName  *uint16
		LpszClassName *uint16
	}
	wc.LpfnWndProc = wndProc
	wc.HInstance = hInstance
	wc.LpszClassName = className
	user32.NewProc("RegisterClassW").Call(uintptr(unsafe.Pointer(&wc)))

	// Fixed Window without resize
	const WS_OVERLAPPEDWINDOW = 0x00CF0000
	const WS_THICKFRAME = 0x00040000
	const WS_MAXIMIZEBOX = 0x00010000
	style := WS_OVERLAPPEDWINDOW &^ (WS_THICKFRAME | WS_MAXIMIZEBOX)
	//

	hwnd, _, _ := user32.NewProc("CreateWindowExW").Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr("MSI Afterburner Profile Switcher Log"))),
		uintptr(style),
		200, 200, uintptr(windowWide), uintptr(windowHeight),
		0, 0, hInstance, 0,
	)

	logWindowHwnd = syscall.Handle(hwnd)
	logWindowOpen = true

	// Set Window Icon
	setWindowIconFromICO(logWindowHwnd, trayicon.IconData)

	// Edit-Control
	editClass := syscall.StringToUTF16Ptr("EDIT")
	editHwnd, _, _ := user32.NewProc("CreateWindowExW").Call(
		0,
		uintptr(unsafe.Pointer(editClass)),
		0,
		0x50210044,
		0, 0, uintptr(windowWide)-19, uintptr(windowHeight)-46,
		hwnd, 0, hInstance, 0,
	)
	logEditHwnd = syscall.Handle(editHwnd)
	setLogFont()

	user32.NewProc("ShowWindow").Call(hwnd, 5)
	user32.NewProc("UpdateWindow").Call(hwnd)

	// Old Logs
	logMutex.Lock()
	appendTextToEdit(strings.ReplaceAll(logBuffer.String(), "\n", "\r\n"))
	logMutex.Unlock()

	// Subscriber
	if windowChannel == nil {
		windowChannel = make(chan string, 100)
		globalLogger.mutex.Lock()
		globalLogger.subscribers = append(globalLogger.subscribers, windowChannel)
		globalLogger.mutex.Unlock()

		go func() {
			for text := range windowChannel {
				if logEditHwnd != 0 {
					appendTextToEdit(text)
				}
			}
		}()
	}

	defer func() {
		if windowChannel != nil {
			globalLogger.mutex.Lock()
			for i, ch := range globalLogger.subscribers {
				if ch == windowChannel {
					globalLogger.subscribers = append(globalLogger.subscribers[:i], globalLogger.subscribers[i+1:]...)
					break
				}
			}
			globalLogger.mutex.Unlock()
			close(windowChannel)
			windowChannel = nil
		}
	}()

	// Message Loop
	var msg struct {
		Hwnd    uintptr
		Message uint32
		WParam  uintptr
		LParam  uintptr
		Time    uint32
		Pt      struct{ X, Y int32 }
	}

	for {
		ret, _, _ := user32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
		user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// Create Font
func setLogFont() {
	if logEditHwnd == 0 {
		return
	}
	CreateFontW := gdi32.NewProc("CreateFontW")
	height := int32(fontSize)
	hFont, _, _ := CreateFontW.Call(
		uintptr(height),
		0,
		0, 0,
		400,
		0, 0, 0,
		0,
		0, 0, 0, 0,
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(fontName))),
	)

	WM_SETFONT := uintptr(0x0030)
	user32.NewProc("SendMessageW").Call(uintptr(logEditHwnd), WM_SETFONT, hFont, 1)
}
