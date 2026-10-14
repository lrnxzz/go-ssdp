//go:build windows

package ssdp

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func systemVersion() string {
	info := windows.RtlGetVersion()

	return fmt.Sprintf("%d.%d.%d", info.MajorVersion, info.MinorVersion, info.BuildNumber)
}
