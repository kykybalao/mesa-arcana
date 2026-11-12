//go:build windows

package main

import (
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
)

// clipboardText lê o texto copiado (Ctrl+C) no Windows.
func clipboardText() (string, bool) {
	const cfUnicodeText = 13
	if r, _, _ := user32.NewProc("OpenClipboard").Call(0); r == 0 {
		return "", false
	}
	defer user32.NewProc("CloseClipboard").Call()
	h, _, _ := user32.NewProc("GetClipboardData").Call(cfUnicodeText)
	if h == 0 {
		return "", false
	}
	p, _, _ := kernel32.NewProc("GlobalLock").Call(h)
	if p == 0 {
		return "", false
	}
	defer kernel32.NewProc("GlobalUnlock").Call(h)
	size, _, _ := kernel32.NewProc("GlobalSize").Call(h)
	n := int(size / 2)
	if n <= 0 || n > 1<<20 {
		return "", false
	}
	buf := unsafe.Slice((*uint16)(unsafe.Pointer(p)), n)
	for i, v := range buf {
		if v == 0 {
			buf = buf[:i]
			break
		}
	}
	return string(utf16.Decode(buf)), true
}
