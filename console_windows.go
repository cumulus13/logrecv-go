// File: console_windows.go
// Author: Hadi Cahyadi <cumulus13@gmail.com>
// Date: 2026-10-28
// Description:
// License: MIT

//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// enableVT turns on ANSI/VT processing for the console (Windows 10+).
func enableVT() bool {
	k := syscall.NewLazyDLL("kernel32.dll")
	get := k.NewProc("GetConsoleMode")
	set := k.NewProc("SetConsoleMode")
	h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		return false
	}
	var m uint32
	if r, _, _ := get.Call(uintptr(h), uintptr(unsafe.Pointer(&m))); r == 0 {
		return false
	}
	if m&0x0004 != 0 {
		return true
	}
	r, _, _ := set.Call(uintptr(h), uintptr(m|0x0004))
	return r != 0
}
