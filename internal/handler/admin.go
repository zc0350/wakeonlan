package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"wol/internal/db"
)

type createAdminRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) listAdmins(w http.ResponseWriter, r *http.Request) {
	admins, err := h.store.ListAdmins()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取管理员列表失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"admins": admins})
}

func (h *Handler) createAdmin(w http.ResponseWriter, r *http.Request) {
	var req createAdminRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "用户名不能为空")
		return
	}
	if len(req.Password) < 6 {
		writeError(w, http.StatusBadRequest, "密码长度至少 6 位")
		return
	}
	if err := h.store.CreateAdmin(req.Username, req.Password); err != nil {
		if errors.Is(err, db.ErrDuplicate) {
			writeError(w, http.StatusConflict, "该用户名已存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "创建管理员失败")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"message": "已创建管理员"})
}

func (h *Handler) deleteAdmin(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的管理员 ID")
		return
	}
	// 禁止删除自己，避免把自己锁在系统外。
	if id == adminID(r) {
		writeError(w, http.StatusBadRequest, "不能删除当前登录的管理员")
		return
	}
	// 禁止删除最后一个管理员。
	count, err := h.store.AdminCount()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "读取管理员数量失败")
		return
	}
	if count <= 1 {
		writeError(w, http.StatusBadRequest, "系统至少需保留一个管理员")
		return
	}
	if err := h.store.DeleteAdmin(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "管理员不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "删除管理员失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "已删除"})
}

type setPasswordRequest struct {
	Password string `json:"password"`
}

// setAdminPassword 由管理员为“其他”管理员直接重置密码（无需旧密码）。
func (h *Handler) setAdminPassword(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "无效的管理员 ID")
		return
	}
	var req setPasswordRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if len(req.Password) < 6 {
		writeError(w, http.StatusBadRequest, "密码长度至少 6 位")
		return
	}
	if err := h.store.SetAdminPassword(id, req.Password); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "管理员不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "设置密码失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "密码已更新"})
}

type changeOwnPasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

// changeOwnPassword 当前管理员修改自己的密码（需提供旧密码）。
func (h *Handler) changeOwnPassword(w http.ResponseWriter, r *http.Request) {
	var req changeOwnPasswordRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if len(req.NewPassword) < 6 {
		writeError(w, http.StatusBadRequest, "新密码长度至少 6 位")
		return
	}
	if err := h.store.ChangePassword(adminID(r), req.OldPassword, req.NewPassword); err != nil {
		if errors.Is(err, db.ErrAuthFailed) {
			writeError(w, http.StatusBadRequest, "原密码错误")
			return
		}
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "管理员不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "修改密码失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "密码已修改"})
}
