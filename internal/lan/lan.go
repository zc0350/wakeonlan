// Package lan 提供局域网设备发现能力。
//
// 原理：向子网内各 IPv4 地址并发发送单个 UDP 探测包，促使操作系统内核
// 发起 ARP 解析（无需目标端口有监听者，也无需 root / 原始套接字权限），
// 稍候片刻再读取系统 ARP 表，即可得到「IP + MAC」的在线设备列表。
package lan

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"wol/internal/model"
)

// Host 是扫描发现的局域网主机。
type Host struct {
	IP       string `json:"ip"`
	MAC      string `json:"mac"`
	Hostname string `json:"hostname,omitempty"`
}

// maxScanHosts 限制单次扫描的主机地址数量，防止误扫超大网段（如 0.0.0.0/0）。
const maxScanHosts = 4096

// DefaultCIDRs 自动检测本机所有非回环 IPv4 地址所在的 /24 网段。
func DefaultCIDRs() ([]string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil, fmt.Errorf("获取本机网络地址失败: %w", err)
	}
	seen := map[string]bool{}
	var out []string
	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipnet.IP.To4()
		if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			continue
		}
		cidr := fmt.Sprintf("%d.%d.%d.%d/24", ip[0], ip[1], ip[2], 0)
		if !seen[cidr] {
			seen[cidr] = true
			out = append(out, cidr)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("未检测到本机局域网 IPv4 地址，请手动指定网段（如 192.168.1.0/24）")
	}
	return out, nil
}

// Scan 扫描给定网段（如 192.168.1.0/24），返回发现的 IP+MAC 列表。
// 结果按 IP 升序排列，同一 MAC 只保留一个条目，组播/广播 MAC 已过滤。
func Scan(cidrs []string) ([]Host, error) {
	var targets []net.IP
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		_, ipnet, err := net.ParseCIDR(c)
		if err != nil {
			return nil, fmt.Errorf("无效网段 %q（示例：192.168.1.0/24）", c)
		}
		if ipnet.IP.To4() == nil {
			return nil, fmt.Errorf("仅支持 IPv4 网段：%s", c)
		}
		hosts := hostIPs(ipnet)
		if len(hosts) > maxScanHosts {
			return nil, fmt.Errorf("网段 %s 包含 %d 个主机地址，超过单次上限 %d，请缩小范围", c, len(hosts), maxScanHosts)
		}
		targets = append(targets, hosts...)
	}
	if len(targets) == 0 {
		return nil, errors.New("网段内没有可扫描的主机地址")
	}

	probeAll(targets)
	// 等待 ARP 应答在内核中汇聚完成。
	time.Sleep(600 * time.Millisecond)

	entries, err := readARP()
	if err != nil {
		return nil, err
	}

	allowed := make(map[string]bool, len(targets))
	for _, ip := range targets {
		allowed[ip.String()] = true
	}

	seenMAC := map[string]bool{}
	var out []Host
	for _, e := range entries {
		if !allowed[e.ip] {
			continue
		}
		mac := model.NormalizeMAC(e.mac)
		if mac == "" || isBroadcastOrMulticastMAC(mac) {
			continue
		}
		if seenMAC[mac] {
			continue
		}
		seenMAC[mac] = true
		out = append(out, Host{IP: e.ip, MAC: mac})
	}

	resolveHostnames(out)
	sort.Slice(out, func(i, j int) bool { return ipLess(out[i].IP, out[j].IP) })
	return out, nil
}

// hostIPs 枚举网段内所有可用主机地址（排除网络地址与广播地址；/31、/32 原样返回）。
func hostIPs(ipnet *net.IPNet) []net.IP {
	base := ipnet.IP.To4()
	ones, _ := ipnet.Mask.Size()
	if ones >= 31 {
		return []net.IP{base}
	}
	network := binary.BigEndian.Uint32(base)
	size := uint64(1) << (32 - ones)
	start := uint64(network) + 1
	end := uint64(network) + size - 2

	out := make([]net.IP, 0, end-start+1)
	for i := start; i <= end; i++ {
		ip := make(net.IP, 4)
		binary.BigEndian.PutUint32(ip, uint32(i))
		out = append(out, ip)
	}
	return out
}

// probeAll 并发向各 IP 的 UDP 9 端口写入 1 字节，触发内核 ARP 解析。
// 目标端口无需有进程监听；即使包被丢弃，ARP 解析也已经发生。
func probeAll(ips []net.IP) {
	sem := make(chan struct{}, 128)
	var wg sync.WaitGroup
	for _, ip := range ips {
		wg.Add(1)
		go func(ip net.IP) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			conn, err := net.DialTimeout("udp", net.JoinHostPort(ip.String(), "9"), 300*time.Millisecond)
			if err != nil {
				return
			}
			// DialTimeout 只约束建连；对不可达路由（如 VPN 虚拟网段），
			// Write 可能长时间阻塞，必须单独设置写超时。
			_ = conn.SetWriteDeadline(time.Now().Add(300 * time.Millisecond))
			_, _ = conn.Write([]byte{0})
			_ = conn.Close()
		}(ip)
	}
	wg.Wait()
}

// resolveHostnames 并发做反向 DNS 解析。
// 每个查询限时 400ms；整个阶段最多等待 1.2s，超时后未完成的主机名留空——
// 局域网内多数设备没有 PTR 记录，DNS 服务器慢响应不应拖慢整体扫描。
func resolveHostnames(hosts []Host) {
	var wg sync.WaitGroup
	for i := range hosts {
		wg.Add(1)
		go func(h *Host) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
			defer cancel()
			// 使用系统解析器（Windows 上可利用 NetBIOS/DNS 缓存拿到主机名），
			// 即使个别查询不受 ctx 控制拖尾，外层 2s 总预算也会保证整体时长。
			names, err := net.DefaultResolver.LookupAddr(ctx, h.IP)
			if err == nil && len(names) > 0 {
				h.Hostname = strings.TrimSuffix(strings.TrimSpace(names[0]), ".")
			}
		}(&hosts[i])
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		// 放弃等待，已解析到的保留，未完成的主机名留空。
	}
}

// isBroadcastOrMulticastMAC 判断是否为广播/组播 MAC（首字节最低位为 1）。
func isBroadcastOrMulticastMAC(mac string) bool {
	first, err := strconv.ParseUint(mac[0:2], 16, 8)
	if err != nil {
		return true
	}
	return first&0x01 == 0x01
}

// ipLess 按 IPv4 数值比较两个点分十进制地址。
func ipLess(a, b string) bool {
	ia, ib := net.ParseIP(a).To4(), net.ParseIP(b).To4()
	if ia == nil || ib == nil {
		return a < b
	}
	return binary.BigEndian.Uint32(ia) < binary.BigEndian.Uint32(ib)
}
