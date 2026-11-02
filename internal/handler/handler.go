package handler

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"errors"
	"net/http"
	"strings"

	"wol/internal/auth"
	"wol/internal/config"
	"wol/internal/db"
)

// 用于在中转上下文中保存当前登录管理员信息的键。
type ctxKey string

const (
	ctxAdminID  ctxKey = "admin_id"
	ctxUsername ctxKey = "username"
)

// Handler 持有依赖并对外提供 HTTP 处理能力。
type Handler struct {
	store  *db.Store
	secret string
	cfg    *config.Config
}

// New 创建一个 Handler。
func New(store *db.Store, cfg *config.Config) *Handler {
	secret := cfg.JWTSecret
	if secret == "" {
		// 未配置密钥时退回一个开发默认值，并在日志中告警（见 main）。
		secret = "dev-insecure-secret-change-me"
	}
	return &Handler{store: store, secret: secret, cfg: cfg}
}

// RegisterRoutes 注册所有路由。webFS 为内嵌的 web 资源。
func (h *Handler) RegisterRoutes(mux *http.ServeMux, webFS embed.FS) {
	// 认证相关
	mux.HandleFunc("POST /api/login", h.login)
	mux.HandleFunc("POST /api/logout", h.logout)
	mux.HandleFunc("GET /api/me", h.authRequired(h.me))

	// 设备相关（均需登录）
	mux.HandleFunc("GET /api/devices", h.authRequired(h.listDevices))
	mux.HandleFunc("POST /api/devices", h.authRequired(h.createDevice))
	mux.HandleFunc("POST /api/devices/batch", h.authRequired(h.batchCreateDevices))
	mux.HandleFunc("PUT /api/devices/{id}", h.authRequired(h.updateDevice))
	mux.HandleFunc("DELETE /api/devices/{id}", h.authRequired(h.deleteDevice))
	mux.HandleFunc("POST /api/devices/{id}/wake", h.authRequired(h.wakeDevice))
	mux.HandleFunc("POST /api/devices/wake", h.authRequired(h.wakeDevices))

	// 局域网扫描（需登录）
	mux.HandleFunc("GET /api/lan/scan", h.authRequired(h.scanLAN))

	// 管理员相关（均需登录）
	mux.HandleFunc("GET /api/admins", h.authRequired(h.listAdmins))
	mux.HandleFunc("POST /api/admins", h.authRequired(h.createAdmin))
	mux.HandleFunc("DELETE /api/admins/{id}", h.authRequired(h.deleteAdmin))
	mux.HandleFunc("PUT /api/admins/{id}/password", h.authRequired(h.setAdminPassword))
	mux.HandleFunc("PUT /api/admins/me/password", h.authRequired(h.changeOwnPassword))

	// 静态资源与页面
	sub, err := fs.Sub(webFS, "web/static")
	if err == nil {
		mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(sub))))
	}
	mux.HandleFunc("GET /login", h.servePage(webFS, "web/templates/login.html"))
	mux.HandleFunc("GET /{$}", h.servePage(webFS, "web/templates/index.html"))
}

// ---------- 辅助函数 ----------

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func adminID(r *http.Request) int64 {
	if v, ok := r.Context().Value(ctxAdminID).(int64); ok {
		return v
	}
	return 0
}

func username(r *http.Request) string {
	if v, ok := r.Context().Value(ctxUsername).(string); ok {
		return v
	}
	return ""
}

func (h *Handler) tokenFromRequest(r *http.Request) string {
	if c, err := r.Cookie("token"); err == nil && c.Value != "" {
		return c.Value
	}
	authz := r.Header.Get("Authorization")
	if strings.HasPrefix(authz, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(authz, "Bearer "))
	}
	return ""
}

// authRequired 是一个中间件：校验登录令牌，并把管理员信息注入上下文。
func (h *Handler) authRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := h.tokenFromRequest(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "未登录或登录已过期")
			return
		}
		claims, err := auth.ParseToken(h.secret, token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "登录已过期，请重新登录")
			return
		}
		ctx := context.WithValue(r.Context(), ctxAdminID, claims.AdminID)
		ctx = context.WithValue(ctx, ctxUsername, claims.Username)
		next(w, r.WithContext(ctx))
	}
}

// servePage 返回一个直接输出内嵌 HTML 页面的处理器。
func (h *Handler) servePage(fs embed.FS, path string) http.HandlerFunc {
	data, err := fs.ReadFile(path)
	if err != nil {
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "页面资源缺失", http.StatusInternalServerError)
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	}
}

// ---------- 认证接口 ----------

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "请求格式错误")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "用户名和密码不能为空")
		return
	}
	admin, err := h.store.Authenticate(req.Username, req.Password)
	if err != nil {
		if errors.Is(err, db.ErrAuthFailed) || errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "用户名或密码错误")
			return
		}
		writeError(w, http.StatusInternalServerError, "登录失败")
		return
	}
	token, err := auth.GenerateToken(h.secret, admin.ID, admin.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "生成令牌失败")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(auth.TokenTTL.Seconds()),
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":    token,
		"username": admin.Username,
	})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"message": "已退出登录"})
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":       adminID(r),
		"username": username(r),
	})
}
