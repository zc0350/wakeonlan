package handler

import (
	"net"
	"net/http"
	"strings"

	"wol/internal/lan"
	"wol/internal/model"
	"wol/internal/wol"
)

// scanLAN 处理 GET /api/lan/scan?cidr=192.168.1.0/24。
// 未指定 cidr 时自动扫描本机所在的所有 /24 网段。
// 返回的每个主机带 exists 标记，表示该 MAC 是否已在设备列表中。
func (h *Handler) scanLAN(w http.ResponseWriter, r *http.Request) {
	cidrParam := strings.TrimSpace(r.URL.Query().Get("cidr"))
	cidrs := []string{cidrParam}
	if cidrParam == "" {
		detected, err := lan.DefaultCIDRs()
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		cidrs = detected
	}

	hosts, err := lan.Scan(cidrs)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// 标记已存在的设备（按规范化 MAC 比较）。
	existing := map[string]bool{}
	if devs, err := h.store.ListDevices(); err == nil {
		for _, d := range devs {
			existing[model.NormalizeMAC(d.MAC)] = true
		}
	}
	type scanHost struct {
		lan.Host
		Exists bool `json:"exists"`
	}
	out := make([]scanHost, 0, len(hosts))
	for _, host := range hosts {
		out = append(out, scanHost{Host: host, Exists: existing[host.MAC]})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"cidrs": cidrs, "hosts": out})
}

type batchDeviceItem struct {
	Name      string `json:"name"`
	MAC       string `json:"mac"`
	Broadcast string `json:"broadcast"`
	Notes     string `json:"notes"`
}

type batchDeviceRequest struct {
	Items []batchDeviceItem `json:"items"`
}

// batchCreateDevices 处理 POST /api/devices/batch，批量添加扫描到的设备。
// MAC 已存在或非法的条目会被跳过；请求体单次最多 500 条。
func (h *Handler) batchCreateDevices(w http.ResponseWriter, r *http.Request) {
	var req batchDeviceRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "未选择任何设备")
		return
	}
	if len(req.Items) > 500 {
		writeError(w, http.StatusBadRequest, "单次最多添加 500 台设备")
		return
	}

	// 已有设备的规范化 MAC 集合，用于跳过重复项。
	existing := map[string]bool{}
	if devs, err := h.store.ListDevices(); err == nil {
		for _, d := range devs {
			existing[model.NormalizeMAC(d.MAC)] = true
		}
	}

	created, skipped := 0, 0
	for _, it := range req.Items {
		mac := model.NormalizeMAC(it.MAC)
		if _, err := net.ParseMAC(it.MAC); err != nil || mac == "" {
			skipped++
			continue
		}
		if existing[mac] {
			skipped++
			continue
		}

		broadcast := strings.TrimSpace(it.Broadcast)
		if broadcast == "" {
			broadcast = wol.DefaultBroadcast
		} else if _, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(broadcast, "9")); err != nil {
			broadcast = wol.DefaultBroadcast // 非法广播地址时回退默认值
		}

		name := strings.TrimSpace(it.Name)
		if name == "" {
			name = mac
		}
		d := &model.Device{
			Name:      name,
			MAC:       mac,
			Broadcast: broadcast,
			Notes:     strings.TrimSpace(it.Notes),
		}
		if _, err := h.store.CreateDevice(d); err != nil {
			skipped++
			continue
		}
		existing[mac] = true
		created++
	}
	writeJSON(w, http.StatusOK, map[string]int{"created": created, "skipped": skipped})
}
