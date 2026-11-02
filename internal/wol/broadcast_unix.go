//go:build !windows

package wol

import "syscall"

// setBroadcast 在非 Windows 平台设置 SO_BROADCAST 套接字选项。
func setBroadcast(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
}
