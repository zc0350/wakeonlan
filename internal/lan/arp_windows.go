//go:build windows

package lan

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// arpEntry 是 ARP 表中的一条记录。
type arpEntry struct {
	ip  string
	mac string
}

// readARP 通过解析 `arp -a` 输出读取 Windows ARP 表。
// 数据行格式：192.168.1.1          aa-bb-cc-dd-ee-ff     动态
// 中文系统的类型列为 GBK 编码可能乱码，但 IP/MAC 为 ASCII 不受影响。
func readARP() ([]arpEntry, error) {
	out, err := exec.Command("arp", "-a").Output()
	if err != nil {
		return nil, fmt.Errorf("执行 arp -a 失败: %w", err)
	}
	re := regexp.MustCompile(
		`^\s*(\d+\.\d+\.\d+\.\d+)\s+([0-9a-fA-F]{2}(?:[-:][0-9a-fA-F]{2}){5})(?:\s+\S+)?\s*$`)
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
