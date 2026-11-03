package main

import (
	"embed"
	"log"
	"net/http"
	"strings"

	"wol/internal/config"
	"wol/internal/db"
	"wol/internal/handler"
)

//go:embed web/templates/*.html web/static/*
var webFS embed.FS

// version 是程序版本号；正式发布时由 CI 通过 -ldflags "-X main.version=..." 注入。
var version = "dev"

func main() {
	cfg := config.Load()

	log.Printf("Wake-on-LAN 管理后台 %s", version)
	log.Printf("数据文件(JSON): %s", cfg.DBPath)
	log.Printf("默认管理员账号: %s / %s (首次启动创建，请尽快修改密码)",
		cfg.DefaultAdmin, cfg.DefaultPassword)

	store, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("初始化数据文件失败: %v", err)
	}
	defer store.Close()

	// JWT 密钥优先级：环境变量 WOOL_JWT_SECRET > 数据文件 server.jwt_secret
	// （数据文件中没有且环境变量未设置时，Open 已自动生成并写回文件）。
	secret := cfg.JWTSecret
	secretSource := "环境变量 WOOL_JWT_SECRET"
	if secret == "" {
		secret = store.JWTSecret()
		secretSource = "数据文件 server.jwt_secret（首次自动生成）"
	}
	if secret == "" {
		log.Fatal("无法确定 JWT 密钥：环境变量与数据文件均为空")
	}
	cfg.JWTSecret = secret
	log.Printf("JWT 密钥来源: %s", secretSource)

	// 监听地址优先级：环境变量 WOOL_LISTEN_ADDR > 数据文件 server.listen > 默认值。
	listen := cfg.ListenAddr
	source := "环境变量 WOOL_LISTEN_ADDR"
	if listen == "" {
		listen = store.ListenAddr()
		source = "数据文件 server.listen"
	}
	if listen == "" {
		listen = config.DefaultListenAddr
		source = "内置默认值"
	}
	// 允许在数据文件里只写端口号（如 "25352"）。
	if !strings.Contains(listen, ":") && isAllDigits(listen) {
		listen = ":" + listen
	}

	log.Printf("监听地址: %s（来源：%s）", listen, source)

	h := handler.New(store, cfg)

	mux := http.NewServeMux()
	h.RegisterRoutes(mux, webFS)

	log.Printf("Wake-on-LAN 管理后台已启动，监听 %s", listen)
	log.Printf("打开浏览器访问: http://localhost%s/", listen)
	if err := http.ListenAndServe(listen, mux); err != nil {
		log.Fatalf("服务启动失败: %v", err)
	}
}

// isAllDigits 判断字符串是否为非空纯数字。
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
