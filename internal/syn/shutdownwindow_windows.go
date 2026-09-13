//go:build windows

package syn

import (
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// A window nobody sees, kept only to be told the session is ending.
//
// The console control handler cannot do this job in the build that autostart
// uses: that binary is linked for the GUI subsystem precisely so Windows
// never gives it a console, and a handler needs a console to be registered
// on. A window does not - and it is the better instrument anyway, because
// ShutdownBlockReasonCreate wants a window handle, and the one
// GetConsoleWindow returns under a modern terminal is not ours to use.
const (
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016

	wsMinimize = 0x20000000
)

var (
	procRegisterClassEx  = user32.NewProc("RegisterClassExW")
	procCreateWindowEx   = user32.NewProc("CreateWindowExW")
	procDefWindowProc    = user32.NewProc("DefWindowProcW")
	procGetMessage       = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessage  = user32.NewProc("DispatchMessageW")
	procGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")

	// shutdownWindow is the handle the block reason is attached to. Zero
	// until the window exists, which is why every use checks.
	shutdownWindow atomic.Uintptr
)

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   uintptr
	icon       uintptr
	cursor     uintptr
	background uintptr
	menuName   uintptr
	className  uintptr
	iconSm     uintptr
}

type winMsg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

// startShutdownWindow creates the window and pumps its messages forever. The
// loop owns its thread: window messages are delivered to the thread that
// created the window, and Go would otherwise move the goroutine elsewhere.
func startShutdownWindow() {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		className, err := syscall.UTF16PtrFromString("synchronize-shutdown")
		if err != nil {
			return
		}
		instance, _, _ := procGetModuleHandle.Call(0)

		proc := syscall.NewCallback(func(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
			switch message {
			case wmQueryEndSession:
				// Windows is asking whether it may close us. Publish first,
				// with the reason on screen, then say yes. Returning FALSE
				// would cancel the shutdown, which is not ours to decide.
				runShutdownHandler()
				return 1
			case wmEndSession:
				// Second chance: some paths skip the question.
				if wParam != 0 {
					runShutdownHandler()
				}
				return 0
			}
			ret, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
			return ret
		})

		class := wndClassEx{
			size:      uint32(unsafe.Sizeof(wndClassEx{})),
			wndProc:   proc,
			instance:  instance,
			className: uintptr(unsafe.Pointer(className)),
		}
		if atom, _, _ := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class))); atom == 0 {
			Debug("could not register the shutdown window class")
			return
		}

		// A real top-level window, never shown. Message-only windows are
		// cheaper but do not receive WM_QUERYENDSESSION, which is the whole
		// point of having one.
		hwnd, _, _ := procCreateWindowEx.Call(
			0,
			uintptr(unsafe.Pointer(className)),
			uintptr(unsafe.Pointer(className)),
			wsMinimize,
			0, 0, 0, 0,
			0, 0, instance, 0,
		)
		if hwnd == 0 {
			Debug("could not create the shutdown window")
			return
		}
		shutdownWindow.Store(hwnd)

		var m winMsg
		for {
			ret, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(ret) <= 0 {
				return
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()
}

// runShutdownHandler is shared by the window and the console handler, so
// whichever hears about the shutdown first does the work once.
func runShutdownHandler() {
	if fn := onShutdown; fn != nil {
		fn()
	}
}

// blockWindow is the handle to hang a shutdown reason on: the hidden window
// when there is one, otherwise whatever console we have.
func blockWindow() uintptr {
	if hwnd := shutdownWindow.Load(); hwnd != 0 {
		return hwnd
	}
	hwnd, _, _ := procGetConsoleWindow.Call()
	return hwnd
}
