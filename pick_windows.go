//go:build windows

package main

import (
	"runtime"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

type openFileName struct {
	lStructSize       uint32
	hwndOwner         uintptr
	hInstance         uintptr
	lpstrFilter       *uint16
	lpstrCustomFilter *uint16
	nMaxCustFilter    uint32
	nFilterIndex      uint32
	lpstrFile         *uint16
	nMaxFile          uint32
	lpstrFileTitle    *uint16
	nMaxFileTitle     uint32
	lpstrInitialDir   *uint16
	lpstrTitle        *uint16
	flags             uint32
	nFileOffset       uint16
	nFileExtension    uint16
	lpstrDefExt       *uint16
	lCustData         uintptr
	lpfnHook          uintptr
	lpTemplateName    *uint16
	pvReserved        unsafe.Pointer
	dwReserved        uint32
	flagsEx           uint32
}

const canPick = true

// pickImage abre a janela "Abrir arquivo" do Windows; devolve "" se a pessoa cancelar
func pickImage() string {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	filter := utf16.Encode([]rune("Imagens (PNG, JPG)\x00*.png;*.jpg;*.jpeg\x00Todos os arquivos\x00*.*\x00\x00"))
	title, _ := syscall.UTF16PtrFromString("Escolher imagem")
	buf := make([]uint16, 4096)
	owner, _, _ := user32.NewProc("GetForegroundWindow").Call()
	ofn := openFileName{
		hwndOwner:   owner,
		lpstrFilter: &filter[0],
		lpstrFile:   &buf[0],
		nMaxFile:    uint32(len(buf)),
		lpstrTitle:  title,
		// EXPLORER | FILEMUSTEXIST | PATHMUSTEXIST | NOCHANGEDIR | HIDEREADONLY
		flags: 0x80000 | 0x1000 | 0x800 | 0x8 | 0x4,
	}
	ofn.lStructSize = uint32(unsafe.Sizeof(ofn))
	r, _, _ := syscall.NewLazyDLL("comdlg32.dll").NewProc("GetOpenFileNameW").Call(uintptr(unsafe.Pointer(&ofn)))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}
