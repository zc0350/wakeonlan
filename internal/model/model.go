package model

import (
	"net"
	"strings"
	"time"
)

// NormalizeMAC 将 MAC 地址规范化为冒号分隔的大写形式（如 AA:BB:CC:DD:EE:FF）。
// 解析失败时原样返回去空格的大写字符串。
func NormalizeMAC(mac string) string {
	mac = strings.TrimSpace(mac)
	parsed, err := net.ParseMAC(mac)
	if err != nil {
		return strings.ToUpper(mac)
	}
	return strings.ToUpper(parsed.String())
}

// EqualMAC 判断两个 MAC 地址是否为同一地址（忽略大小写与分隔符差异）。
func EqualMAC(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return NormalizeMAC(a) == NormalizeMAC(b)
}

// Device 表示局域网中被管理的设备。
type Device struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	MAC       string    `json:"mac"`
	Broadcast string    `json:"broadcast"` // 唤醒包发送到的广播地址，默认 255.255.255.255
	Notes     string    `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Admin 表示平台管理员。密码哈希不会出现在该结构体中，避免泄露。
type Admin struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"created_at"`
}
