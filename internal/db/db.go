package db

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"wol/internal/config"
	"wol/internal/model"
	"wol/internal/wol"

	"golang.org/x/crypto/bcrypt"
)

// adminRecord 是带密码哈希的内部记录，仅用于数据文件读写。
type adminRecord struct {
	model.Admin
	PasswordHash string `json:"password_hash"`
}

// ServerConfig 是数据文件中可选的服务器配置段。
type ServerConfig struct {
	// Listen 是 HTTP 监听地址，如 ":25352" 或纯端口号 "25352"。
	Listen string `json:"listen"`
	// JWTSecret 是登录令牌的签名密钥；为空时会在启动时自动生成并写回本文件。
	JWTSecret string `json:"jwt_secret,omitempty"`
	// WakeAttempts 是每台设备唤醒魔术包的发送次数；缺省/非法值会在启动时补写为默认值 3。
	WakeAttempts int `json:"wake_attempts"`
	// WakeIntervalMS 是两次唤醒包之间的间隔（毫秒）；缺省/非法值会在启动时补写为默认值 100。
	WakeIntervalMS int `json:"wake_interval_ms"`
}

// fileData 是 JSON 数据文件的顶层结构。
type fileData struct {
	Server  *ServerConfig  `json:"server,omitempty"`
	Devices []model.Device `json:"devices"`
	Admins  []adminRecord  `json:"admins"`
}

// Store 基于单个 JSON 文件的存储层。适用于小数据量场景：
//   - 全部数据常驻内存，写操作即时落盘（全量重写）；
//   - 落盘采用"临时文件 + rename 原子替换"，崩溃不会损坏数据；
//   - 每次落盘前把上一版备份为 <path>.bak，便于误改后恢复；
//   - 所有方法经互斥锁串行化，规避 HTTP 并发读写竞争。
type Store struct {
	mu   sync.Mutex
	path string
	data fileData
}

// Open 读取（必要时创建）JSON 数据文件，并在没有任何管理员时初始化默认账号。
func Open(cfg *config.Config) (*Store, error) {
	s := &Store{
		path: cfg.DBPath,
		data: fileData{
			Devices: []model.Device{},
			Admins:  []adminRecord{},
		},
	}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("读取数据文件 %s: %w", s.path, err)
		}
		// 文件不存在：全新安装，使用空数据。
	} else if err := json.Unmarshal(raw, &s.data); err != nil {
		return nil, fmt.Errorf("解析数据文件 %s 失败（文件可能被手工改坏，可尝试恢复同目录的 .bak）: %w", s.path, err)
	}

	// 确保切片非 nil，落盘时输出 [] 而不是 null。
	if s.data.Devices == nil {
		s.data.Devices = []model.Device{}
	}
	if s.data.Admins == nil {
		s.data.Admins = []adminRecord{}
	}

	// 补写 server 配置段，让监听端口在数据文件中可见、可手工修改。
	needSave := false
	if s.data.Server == nil {
		s.data.Server = &ServerConfig{Listen: config.DefaultListenAddr}
		needSave = true
	}

	// 补写唤醒发包配置，让次数/间隔在数据文件中可见、可手工修改。
	// 0 或负值视为未配置；启动时统一补写为默认值。
	if s.data.Server.WakeAttempts <= 0 {
		s.data.Server.WakeAttempts = wol.DefaultAttempts
		needSave = true
	}
	if s.data.Server.WakeIntervalMS <= 0 {
		s.data.Server.WakeIntervalMS = int(wol.DefaultInterval / time.Millisecond)
		needSave = true
	}

	// JWT 密钥：环境变量未提供且文件中没有时，生成随机密钥并持久化，
	// 保证密钥跨重启稳定（重启后已登录用户无需重新登录）。
	if cfg.JWTSecret == "" && s.data.Server.JWTSecret == "" {
		sec, err := generateSecret()
		if err != nil {
			return nil, fmt.Errorf("生成 JWT 密钥: %w", err)
		}
		s.data.Server.JWTSecret = sec
		needSave = true
	}

	if len(s.data.Admins) == 0 {
		if err := s.CreateAdmin(cfg.DefaultAdmin, cfg.DefaultPassword); err != nil {
			return nil, fmt.Errorf("创建默认管理员: %w", err)
		}
		needSave = false // CreateAdmin 内部已落盘
	}

	if needSave {
		s.mu.Lock()
		if err := s.save(); err != nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("写入数据文件: %w", err)
		}
		s.mu.Unlock()
	}
	return s, nil
}

// ListenAddr 返回数据文件中配置的监听地址（server.listen），未配置时返回空串。
func (s *Store) ListenAddr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Server == nil {
		return ""
	}
	return s.data.Server.Listen
}

// JWTSecret 返回数据文件中持久化的 JWT 密钥；为空表示应由环境变量提供。
func (s *Store) JWTSecret() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Server == nil {
		return ""
	}
	return s.data.Server.JWTSecret
}

// WakeConfig 返回唤醒发包配置（发送次数与间隔毫秒）。
// 文件中缺省/非法（<=0）的值回退为默认值；上限防呆由 wol.Options 统一处理。
func (s *Store) WakeConfig() (attempts int, intervalMS int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	attempts = s.data.Server.WakeAttempts
	if attempts <= 0 {
		attempts = wol.DefaultAttempts
	}
	intervalMS = s.data.Server.WakeIntervalMS
	if intervalMS <= 0 {
		intervalMS = int(wol.DefaultInterval / time.Millisecond)
	}
	return attempts, intervalMS
}

// generateSecret 生成 64 字符的十六进制随机串，用作 JWT 签名密钥。
func generateSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// Close 对 JSON 存储无需处理（写操作均已即时落盘）。
func (s *Store) Close() error { return nil }

// save 把内存数据原子写入磁盘。调用方必须已持有 s.mu。
func (s *Store) save() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化数据: %w", err)
	}
	raw = append(raw, '\n')

	// 备份上一版数据（尽力而为，失败不影响主流程）。
	if bak, err := os.ReadFile(s.path); err == nil {
		_ = os.WriteFile(s.path+".bak", bak, 0o600)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("写入临时文件: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("原子替换数据文件: %w", err)
	}
	return nil
}

// nextAdminID 返回下一个可用的管理员 ID。调用方必须已持有 s.mu。
func (s *Store) nextAdminID() int64 {
	var max int64
	for _, a := range s.data.Admins {
		if a.ID > max {
			max = a.ID
		}
	}
	return max + 1
}

// nextDeviceID 返回下一个可用的设备 ID。调用方必须已持有 s.mu。
func (s *Store) nextDeviceID() int64 {
	var max int64
	for _, d := range s.data.Devices {
		if d.ID > max {
			max = d.ID
		}
	}
	return max + 1
}

// ---------- 管理员 ----------

// CreateAdmin 创建一个新管理员，密码会被 bcrypt 哈希后存储。
// 用户名重复时返回 ErrDuplicate。
func (s *Store) CreateAdmin(username, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, a := range s.data.Admins {
		if a.Username == username {
			return ErrDuplicate
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	s.data.Admins = append(s.data.Admins, adminRecord{
		Admin: model.Admin{
			ID:        s.nextAdminID(),
			Username:  username,
			CreatedAt: time.Now().UTC(),
		},
		PasswordHash: string(hash),
	})
	return s.save()
}

// ListAdmins 返回所有管理员（不含密码）。
func (s *Store) ListAdmins() ([]model.Admin, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]model.Admin, 0, len(s.data.Admins))
	for _, a := range s.data.Admins {
		out = append(out, a.Admin)
	}
	return out, nil
}

func (s *Store) findAdminByUsernameLocked(username string) (*adminRecord, int, bool) {
	for i, a := range s.data.Admins {
		if a.Username == username {
			return &s.data.Admins[i], i, true
		}
	}
	return nil, 0, false
}

func (s *Store) findAdminByIDLocked(id int64) (*adminRecord, int, bool) {
	for i, a := range s.data.Admins {
		if a.ID == id {
			return &s.data.Admins[i], i, true
		}
	}
	return nil, 0, false
}

// Authenticate 校验用户名/密码，成功返回管理员信息。
func (s *Store) Authenticate(username, password string) (*model.Admin, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, _, ok := s.findAdminByUsernameLocked(username)
	if !ok {
		return nil, ErrNotFound
	}
	if err := bcrypt.CompareHashAndPassword([]byte(r.PasswordHash), []byte(password)); err != nil {
		return nil, ErrAuthFailed
	}
	admin := r.Admin
	return &admin, nil
}

// DeleteAdmin 删除指定管理员。
func (s *Store) DeleteAdmin(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, i, ok := s.findAdminByIDLocked(id)
	if !ok {
		return ErrNotFound
	}
	s.data.Admins = append(s.data.Admins[:i], s.data.Admins[i+1:]...)
	return s.save()
}

// AdminCount 返回管理员总数。
func (s *Store) AdminCount() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.data.Admins), nil
}

// SetAdminPassword 直接设置某管理员的密码（无需旧密码）。
func (s *Store) SetAdminPassword(id int64, newPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setPasswordLocked(id, newPassword)
}

// setPasswordLocked 设置密码并落盘。调用方必须已持有 s.mu。
func (s *Store) setPasswordLocked(id int64, newPassword string) error {
	r, _, ok := s.findAdminByIDLocked(id)
	if !ok {
		return ErrNotFound
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	r.PasswordHash = string(hash)
	return s.save()
}

// ChangePassword 校验旧密码后修改指定管理员的密码。
func (s *Store) ChangePassword(id int64, oldPassword, newPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, _, ok := s.findAdminByIDLocked(id)
	if !ok {
		return ErrNotFound
	}
	if err := bcrypt.CompareHashAndPassword([]byte(r.PasswordHash), []byte(oldPassword)); err != nil {
		return ErrAuthFailed
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	r.PasswordHash = string(hash)
	return s.save()
}

// ---------- 设备 ----------

// CreateDevice 创建一台设备。
func (s *Store) CreateDevice(d *model.Device) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	d.ID = s.nextDeviceID()
	d.CreatedAt = now
	d.UpdatedAt = now
	s.data.Devices = append(s.data.Devices, *d)
	return d.ID, s.save()
}

// ListDevices 返回全部设备，按名称排序（不区分大小写）。
func (s *Store) ListDevices() ([]model.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]model.Device, len(s.data.Devices))
	copy(out, s.data.Devices)
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (s *Store) findDeviceByIDLocked(id int64) (*model.Device, bool) {
	for i := range s.data.Devices {
		if s.data.Devices[i].ID == id {
			return &s.data.Devices[i], true
		}
	}
	return nil, false
}

// GetDevice 按 ID 获取设备。
func (s *Store) GetDevice(id int64) (*model.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	d, ok := s.findDeviceByIDLocked(id)
	if !ok {
		return nil, ErrNotFound
	}
	out := *d
	return &out, nil
}

// UpdateDevice 更新设备信息（保留原创建时间）。
func (s *Store) UpdateDevice(d *model.Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.findDeviceByIDLocked(d.ID)
	if !ok {
		return ErrNotFound
	}
	stored.Name = d.Name
	stored.MAC = d.MAC
	stored.Broadcast = d.Broadcast
	stored.Notes = d.Notes
	stored.UpdatedAt = time.Now().UTC()

	// 回填 ID 与创建时间，让调用方拿到完整数据。
	d.ID = stored.ID
	d.CreatedAt = stored.CreatedAt
	d.UpdatedAt = stored.UpdatedAt
	return s.save()
}

// DeleteDevice 删除设备。
func (s *Store) DeleteDevice(id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	i := s.findDeviceIndexLocked(id)
	if i < 0 {
		return ErrNotFound
	}
	s.data.Devices = append(s.data.Devices[:i], s.data.Devices[i+1:]...)
	return s.save()
}

// findDeviceIndexLocked 返回设备下标，未找到返回 -1。
func (s *Store) findDeviceIndexLocked(id int64) int {
	for i := range s.data.Devices {
		if s.data.Devices[i].ID == id {
			return i
		}
	}
	return -1
}

// GetDevicesByIDs 批量获取设备，按传入 ID 的顺序返回，忽略不存在的 ID。
func (s *Store) GetDevicesByIDs(ids []int64) ([]model.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]model.Device, 0, len(ids))
	for _, id := range ids {
		if d, ok := s.findDeviceByIDLocked(id); ok {
			out = append(out, *d)
		}
	}
	return out, nil
}
