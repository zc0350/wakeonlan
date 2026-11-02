//go:build windows

package wol

import "syscall"

// setBroadcast 在 Windows 平台设置 SO_BROADCAST 套接字选项。
func setBroadcast(fd uintptr) error {
	return syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
}
