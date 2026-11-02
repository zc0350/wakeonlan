package handler

import (
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"

	"wol/internal/db"
	"wol/internal/model"
	"wol/internal/wol"
)

// deviceRequest 是创建设备/更新设备时的请求体。
type deviceRequest struct {
	Name      string `json:"name"`
	MAC       string `json:"mac"`
	Broadcast string `json:"broadcast"`
	Notes     string `json:"notes"`
}

// validate 校验设备字段，返回规范化后的数据或错误。
func (r *deviceRequest) validate() error {
	r.Name = strings.TrimSpace(r.Name)
	r.MAC = strings.TrimSpace(r.MAC)
	r.Broadcast = strings.TrimSpace(r.Broadcast)
	if r.Name == "" {
		return errors.New("设备名称不能为空")
	}
	if _, err := net.ParseMAC(r.MAC); err != nil {
		return errors.New("MAC 地址格式不正确")
	}
	if r.Broadcast == "" {
		r.Broadcast = wol.DefaultBroadcast
	}
	if _, err := net.ResolveUDPAddr("udp4", net.JoinHostPort(r.Broadcast, "9")); err != nil {
		return errors.New("广播地址格式不正确")
	}
	return nil
}

func (h *Handler) listDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := h.store.ListDevices()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取设备列表失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"devices": devices})
}

func (h *Handler) createDevice(w http.ResponseWriter, r *http.Request) {
	var req deviceRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	d := &model.Device{
		Name:      req.Name,
		MAC:       req.MAC,
		Broadcast: req.Broadcast,
		Notes:     req.Notes,
	}
	id, err := h.store.CreateDevice(d)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "创建设备失败")
		return
	}
	d.ID = id
	writeJSON(w, http.StatusCreated, d)
}

func (h *Handler) updateDevice(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的设备 ID")
		return
	}
	var req deviceRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if err := req.validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	d := &model.Device{
		ID:        id,
		Name:      req.Name,
		MAC:       req.MAC,
		Broadcast: req.Broadcast,
		Notes:     req.Notes,
	}
	if err := h.store.UpdateDevice(d); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "设备不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "更新设备失败")
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (h *Handler) deleteDevice(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的设备 ID")
		return
	}
	if err := h.store.DeleteDevice(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "设备不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "删除设备失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "已删除"})
}

// wakeResult 描述单台设备的唤醒结果。
type wakeResult struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	MAC   string `json:"mac"`
	Error string `json:"error,omitempty"`
}

func (h *Handler) wakeDevice(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的设备 ID")
		return
	}
	d, err := h.store.GetDevice(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "设备不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "读取设备失败")
		return
	}
	res := wakeResult{ID: d.ID, Name: d.Name, MAC: d.MAC}
	if err := wol.Wake(d.MAC, d.Broadcast); err != nil {
		res.Error = err.Error()
	}
	writeJSON(w, http.StatusOK, res)
}

type wakeRequest struct {
	IDs []int64 `json:"ids"`
}

func (h *Handler) wakeDevices(w http.ResponseWriter, r *http.Request) {
	var req wakeRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if len(req.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "未选择任何设备")
		return
	}
	devices, err := h.store.GetDevicesByIDs(req.IDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取设备失败")
		return
	}
	results := make([]wakeResult, 0, len(devices))
	for _, d := range devices {
		res := wakeResult{ID: d.ID, Name: d.Name, MAC: d.MAC}
		if err := wol.Wake(d.MAC, d.Broadcast); err != nil {
			res.Error = err.Error()
		}
		results = append(results, res)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"results": results})
}
