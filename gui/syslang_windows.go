//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// systemLanguage returns the user's preferred UI language as a BCP-47 tag such
// as "ko-KR", or "" when Windows will not say.
//
// GetUserDefaultLocaleName is asked rather than the older LANGID call because it
// hands back the tag directly, and the tag is what the window's own detection
// compares against — one notion of "which language" across both sides.
func systemLanguage() string {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := kernel32.NewProc("GetUserDefaultLocaleName")
	if err := proc.Find(); err != nil {
		return ""
	}
	// LOCALE_NAME_MAX_LENGTH is 85 wide characters, including the terminator.
	buf := make([]uint16, 85)
	n, _, _ := proc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n <= 1 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n-1]) // n counts the terminating null
}
