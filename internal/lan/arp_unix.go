//go:build !windows

package lan

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// arpEntry 是 ARP 表中的一条记录。
type arpEntry struct {
	ip  string
	mac string
}

// readARP 读取系统 ARP 表：优先读 /proc/net/arp（Linux），
// 失败时回退到解析 arp -an 输出（其他 Unix）。
func readARP() ([]arpEntry, error) {
	if entries, err := readProcARP(); err == nil {
		return entries, nil
	}
	return parseArpCommand()
}

// readProcARP 解析 Linux 的 /proc/net/arp。
// 表项 Flags 为 0x2 表示完整（已解析到 MAC），0x0 表示未完成，需过滤。
func readProcARP() ([]arpEntry, error) {
	data, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		return nil, err
	}
	var out []arpEntry
	for i, line := range strings.Split(string(data), "\n") {
		if i == 0 { // 首行为表头
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		ip, flags, mac := fields[0], fields[2], fields[3]
		if flags != "0x2" {
			continue
		}
		if net.ParseIP(ip) == nil {
			continue
		}
		if _, err := net.ParseMAC(mac); err != nil {
			continue
		}
		out = append(out, arpEntry{ip: ip, mac: mac})
	}
	return out, nil
}

// parseArpCommand 解析 `arp -an` 输出（BSD/macOS 风格）：
// ? (192.168.1.1) at aa:bb:cc:dd:ee:ff on eth0
func parseArpCommand() ([]arpEntry, error) {
	out, err := exec.Command("arp", "-an").Output()
	if err != nil {
		return nil, fmt.Errorf("读取 ARP 表失败（/proc/net/arp 与 arp 命令均不可用）: %w", err)
	}
	re := regexp.MustCompile(`\((\d+\.\d+\.\d+\.\d+)\)\s+at\s+([0-9a-fA-F]{1,2}(?::[0-9a-fA-F]{1,2}){5})`)
	var entries []arpEntry
	for _, line := range strings.Split(string(out), "\n") {
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		entries = append(entries, arpEntry{ip: m[1], mac: m[2]})
	}
	return entries, nil
}
