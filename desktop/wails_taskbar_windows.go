//go:build windows && wails && (amd64 || arm64)

package desktop

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"
)

// taskbarVtbl is ITaskbarList3's vtable, in order. Wails's w32 package
// creates the object and calls SetOverlayIcon, but keeps the vtable
// unexported, so the two progress methods are called through this copy of
// its layout: a COM object's first word is its vtable pointer.
type taskbarVtbl struct {
	QueryInterface, AddRef, Release, HrInit, AddTab, DeleteTab, ActivateTab, SetActiveAlt uintptr
	MarkFullscreenWindow, SetProgressValue, SetProgressState                              uintptr
}

// windowsTaskbar draws progress on every window's taskbar button through
// ITaskbarList3, on the main thread as COM's apartment needs.
type windowsTaskbar struct {
	app  *application.App
	list *w32.ITaskbarList3
	err  error
	made bool
}

func newPlatformTaskbar(app *application.App) Taskbar { return &windowsTaskbar{app: app} }

// each runs f for every window's handle on the main thread, creating the
// ITaskbarList3 on first use. It goes on past a window that fails and
// returns every failure.
func (t *windowsTaskbar) each(method string, f func(vtbl *taskbarVtbl, this, hwnd uintptr) uintptr) error {
	return application.InvokeSyncWithError(func() error {
		if !t.made {
			t.made = true
			t.list, t.err = w32.NewTaskbarList3()
		}
		if t.err != nil {
			return fmt.Errorf("ITaskbarList3: %w", t.err)
		}
		this := uintptr(unsafe.Pointer(t.list))
		vtbl := *(**taskbarVtbl)(unsafe.Pointer(t.list))
		var errs []error
		for _, w := range t.app.Window.GetAll() {
			hwnd := uintptr(w.NativeWindow())
			if hwnd == 0 {
				continue
			}
			if hr := uint32(f(vtbl, this, hwnd)); hr != 0 { // S_OK is 0
				errs = append(errs, hresultError(method, w.Name(), hr))
			}
		}
		return errors.Join(errs...)
	})
}

// hresultError describes a failed COM call by its HRESULT.
func hresultError(method, window string, hr uint32) error {
	return fmt.Errorf("ITaskbarList3.%s on window %q: HRESULT 0x%08X", method, window, hr)
}

func (t *windowsTaskbar) SetProgressState(state TaskbarState) error {
	return t.each("SetProgressState", func(v *taskbarVtbl, this, hwnd uintptr) uintptr {
		r, _, _ := syscall.SyscallN(v.SetProgressState, this, hwnd, uintptr(state))
		return r
	})
}

// SetProgressValue passes each ULONGLONG in one register, as the 64-bit
// Windows calling conventions do; the file builds only for amd64 and arm64.
func (t *windowsTaskbar) SetProgressValue(completed, total uint64) error {
	return t.each("SetProgressValue", func(v *taskbarVtbl, this, hwnd uintptr) uintptr {
		r, _, _ := syscall.SyscallN(v.SetProgressValue, this, hwnd, uintptr(completed), uintptr(total))
		return r
	})
}
