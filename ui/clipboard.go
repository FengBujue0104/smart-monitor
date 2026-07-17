package ui

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WriteText 把 s 写入剪贴板（CF_UNICODETEXT）。失败返回 err。
func WriteText(s string) error {
	p, err := windows.UTF16FromString(s)
	if err != nil {
		return err
	}
	const (
		MEM_COMMIT     = 0x1000
		MEM_RELEASE    = 0x8000
		GMEM_MOVEABLE  = 0x0002
		CF_UNICODETEXT = 13
	)

	user32 := windows.NewLazySystemDLL("user32.dll")
	kernel32 := windows.NewLazySystemDLL("kernel32.dll")

	open := user32.NewProc("OpenClipboard")
	empty := user32.NewProc("EmptyClipboard")
	set := user32.NewProc("SetClipboardData")
	close := user32.NewProc("CloseClipboard")
	galloc := kernel32.NewProc("GlobalAlloc")
	gloc := kernel32.NewProc("GlobalLock")
	gunloc := kernel32.NewProc("GlobalUnlock")
	gfree := kernel32.NewProc("GlobalFree")

	ret, _, err := open.Call(0)
	if ret == 0 {
		return err
	}
	defer close.Call(0)

	if r, _, _ := empty.Call(0); r == 0 {
		return syscall.EINVAL
	}

	size := uintptr(len(p) * 2)
	h, _, err := galloc.Call(GMEM_MOVEABLE, size)
	if h == 0 {
		return err
	}
	lp, _, _ := gloc.Call(h)
	if lp == 0 {
		gfree.Call(h)
		return syscall.EINVAL
	}
	// 通过 Win32 内存复制函数写入 GlobalLock 返回的地址，避免把外部地址伪装成 Go slice。
	moveMemory := kernel32.NewProc("RtlMoveMemory")
	moveMemory.Call(lp, uintptr(unsafe.Pointer(&p[0])), size)
	gunloc.Call(h)
	r, _, _ := set.Call(CF_UNICODETEXT, h)
	if r == 0 {
		gfree.Call(h)
		return syscall.EINVAL
	}
	// SetClipboardData 后系统拥有 h，不要再次 GlobalFree
	return nil
}
