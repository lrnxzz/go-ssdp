//go:build unix

package ssdp

import "golang.org/x/sys/unix"

func systemVersion() string {
	var name unix.Utsname
	if err := unix.Uname(&name); err != nil {
		return ""
	}

	return unix.ByteSliceToString(name.Release[:])
}
