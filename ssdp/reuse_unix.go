//go:build unix

package ssdp

import "syscall"

func reuseAddress(_, _ string, conn syscall.RawConn) error {
	var optionErr error

	if err := conn.Control(func(fd uintptr) {
		optionErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	}); err != nil {
		return err
	}

	return optionErr
}
