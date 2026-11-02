package config

import (
	"os"
)

// DefaultListenAddr 是未做任何配置时的默认监听地址。
const DefaultListenAddr = ":25352"

// Config 保存服务的运行时配置，全部可通过环境变量覆盖。
type Config struct {
	DBPath     string // JSON 数据文件路径（含设备与管理员）
	ListenAddr string // HTTP 监听地址；留空表示未通过环境变量设置，将回退到数据文件的 server.listen
	JWTSecret  string // 用于签发登录令牌的密钥
	DefaultAdmin    string // 首次启动时创建的管理员用户名
	DefaultPassword string // 首次启动时创建的管理员密码
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Load 从环境变量读取配置，未设置时使用安全的默认值。
func Load() *Config {
	// 数据文件路径：优先 WOOL_DATA_PATH；兼容旧变量名 WOOL_DB_PATH。
	dataPath := getEnv("WOOL_DATA_PATH", getEnv("WOOL_DB_PATH", "wol.json"))
	return &Config{
		DBPath:          dataPath,
		ListenAddr:      getEnv("WOOL_LISTEN_ADDR", ""),
		JWTSecret:       getEnv("WOOL_JWT_SECRET", ""),
		DefaultAdmin:    getEnv("WOOL_DEFAULT_ADMIN", "admin"),
		DefaultPassword: getEnv("WOOL_DEFAULT_PASSWORD", "admin123"),
	}
}
