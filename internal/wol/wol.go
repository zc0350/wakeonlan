// Package wol 负责构造并发送 Wake-on-LAN 魔术包。
package wol

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// DefaultBroadcast 是魔术包默认发送到的广播地址。
const DefaultBroadcast = "255.255.255.255"

// DefaultPort 是 Wake-on-LAN 使用的 UDP 端口（9 为常用端口，部分设备也用 7）。
const DefaultPort = 9

// Wake 向指定广播地址发送目标 MAC 的唤醒魔术包。
// 为保证可靠性，会连续发送若干次。
func Wake(macStr, broadcast string) error {
	mac, err := net.ParseMAC(strings.TrimSpace(macStr))
	if err != nil {
		return fmt.Errorf("invalid MAC address %q: %w", macStr, err)
	}
	if broadcast == "" {
		broadcast = DefaultBroadcast
	}

	packet, err := buildMagicPacket(mac)
	if err != nil {
		return err
	}

	addr, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(broadcast, fmt.Sprintf("%d", DefaultPort)))
	if err != nil {
		return fmt.Errorf("resolve broadcast address: %w", err)
	}

	// 使用 DialUDP 建立到广播地址的连接，并设置 SO_BROADCAST 以便发送到 255.255.255.255。
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return fmt.Errorf("dial broadcast: %w", err)
	}
	defer conn.Close()

	// 通过底层 socket 选项 SO_BROADCAST 允许向广播地址（255.255.255.255 或子网广播）发包。
	if raw, err := conn.SyscallConn(); err == nil {
		raw.Control(func(fd uintptr) {
			_ = setBroadcast(fd)
		})
	}

	const attempts = 3
	var lastErr error
	for i := 0; i < attempts; i++ {
		if _, err := conn.Write(packet); err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}
		lastErr = nil
		time.Sleep(100 * time.Millisecond)
	}
	if lastErr != nil {
		return fmt.Errorf("send magic packet: %w", lastErr)
	}
	return nil
}

// buildMagicPacket 构造 6 字节 0xFF 前缀 + 16 次重复 MAC 的 102 字节魔术包。
func buildMagicPacket(mac net.HardwareAddr) ([]byte, error) {
	if len(mac) != 6 {
		return nil, fmt.Errorf("MAC must be 6 bytes, got %d", len(mac))
	}
	packet := make([]byte, 6+16*6)
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}
	for i := 0; i < 16; i++ {
		copy(packet[6+i*6:], mac)
	}
	return packet, nil
}
