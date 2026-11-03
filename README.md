# Wake-on-LAN 管理后台

[![ci](https://github.com/zc0350/wakeonlan/actions/workflows/ci.yml/badge.svg)](https://github.com/zc0350/wakeonlan/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/zc0350/wakeonlan)](https://github.com/zc0350/wakeonlan/releases)

基于 Go 的局域网设备远程唤醒（Wake-on-LAN）Web 管理平台。

**单二进制、零外部依赖**：前端页面内嵌于二进制，数据存储为单个 JSON 文件，编译后拷贝到服务器即可运行，无需安装数据库或任何运行时。

## 功能特性

- **设备管理**：添加 / 编辑 / 删除局域网设备（名称、MAC 地址、广播地址、备注）
- **远程唤醒**：单台唤醒，或多选批量唤醒，结果逐台回显
- **局域网扫描**：一键扫描服务器所在网段，发现在线设备的 IP + MAC + 主机名，勾选后批量添加（自动跳过已存在设备），无需 root
- **多管理员**：添加 / 删除管理员、重置他人密码（无需旧密码）、修改自己的密码（需验证旧密码）
- **安全机制**：JWT 登录态、bcrypt 密码哈希、JWT 密钥首次启动自动生成并持久化、文件权限 0600
- **灵活配置**：监听端口、JWT 密钥、唤醒发包次数/间隔均可写入 `wol.json`，环境变量可覆盖
- **自动防呆**：禁止删除自己、禁止删除最后一个管理员、MAC 地址自动规范化、非法配置启动时自愈

## 快速开始

### 方式一：下载预编译二进制（推荐）

从 [Releases](https://github.com/zc0350/wakeonlan/releases) 下载对应平台的压缩包并解压：

```bash
# Linux
tar xzf wol-server-linux-amd64.tar.gz
./wol-server-linux-amd64
```

Windows 下载 `wol-server-windows-amd64.zip` 解压后运行 `wol-server-windows-amd64.exe`。

### 方式二：源码编译

需要 Go 1.26+：

```bash
git clone https://github.com/zc0350/wakeonlan.git
cd wakeonlan
go build -trimpath -ldflags="-s -w" -o wol-server .
```

启动后浏览器访问 `http://<服务器IP>:25352/`。

**默认管理员账号：`admin` / `admin123`**（首次启动自动创建，登录后请立即修改）。

## 配置

所有配置集中在数据文件 `wol.json`（首次启动自动生成）：

```json
{
  "server": {
    "listen": ":25352",
    "jwt_secret": "64位自动生成的随机串",
    "wake_attempts": 3,
    "wake_interval_ms": 100
  },
  "devices": [],
  "admins": []
}
```

| 字段 | 说明 | 默认值 |
|---|---|---|
| `server.listen` | HTTP 监听地址，支持 `":25352"` 或纯端口号 `"25352"` | `:25352` |
| `server.jwt_secret` | 登录令牌签名密钥；缺失时启动自动生成 64 位随机串并写回 | 自动生成 |
| `server.wake_attempts` | 每台设备魔术包发送次数（1~20） | `3` |
| `server.wake_interval_ms` | 两次发包间隔毫秒数（0~5000） | `100` |
| `devices` / `admins` | 设备与管理员数据，由 Web 界面维护 | — |

**优先级**：环境变量 > `wol.json` > 内置默认值。修改 `wol.json` 后需重启服务生效。

支持的环境变量：

| 变量 | 说明 |
|---|---|
| `WOOL_LISTEN_ADDR` | 监听地址（覆盖 `server.listen`） |
| `WOOL_JWT_SECRET` | JWT 密钥（覆盖 `server.jwt_secret`，此时不落盘） |
| `WOOL_DATA_PATH` | 数据文件路径（默认 `wol.json`；兼容旧名 `WOOL_DB_PATH`） |
| `WOOL_DEFAULT_ADMIN` / `WOOL_DEFAULT_PASSWORD` | 首次启动创建的默认管理员账号/密码 |

> `server.listen` 与 `server.jwt_secret` 也可完全留空，启动时自动补写默认值。

## Linux 部署（systemd）

推荐的目录布局：二进制 `/usr/local/bin/wol-server`，数据目录 `/var/local/wol/`。

### 一键部署

```bash
# 把 deploy/ 目录上传到服务器后
sudo bash deploy/install.sh
sudo systemctl status wol-server
```

脚本会创建 `wol` 系统用户、安装二进制与服务、注册开机自启。

### 手动部署

```bash
sudo cp wol-server-linux-amd64 /usr/local/bin/wol-server
sudo mkdir -p /var/local/wol && sudo chown wol:wol /var/local/wol
sudo cp deploy/wol-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now wol-server
```

## 使用指南

| 页签 | 功能 |
|---|---|
| **设备管理** | 设备列表（按名称排序）；「+ 添加设备」手工录入；勾选多台后「批量唤醒选中」；每行「唤醒」按钮单台唤醒，结果逐台显示 |
| **扫描局域网** | 打开弹窗即自动扫描服务器所在的所有 /24 网段（可在输入框指定如 `192.168.1.0/24`）；表格展示 IP / MAC / 主机名，已在设备列表中的默认不勾选；「添加选中设备」一键入库 |
| **管理员管理** | 添加/删除管理员；「重置密码」直接设置他人新密码；不能删除自己或最后一个管理员 |
| **修改密码** | 顶栏入口，需验证旧密码 |

唤醒原理：向设备广播地址的 UDP 9 端口发送 102 字节标准魔术包（6×`0xFF` + 16×MAC），默认连发 3 次、间隔 100ms（可在 `wol.json` 调整）。

### 设备唤醒不了？

1. **BIOS/UEFI**：开启 "Wake on LAN" / "Power On by PCI-E Device"；
2. **操作系统**：Windows 需在网卡属性中勾选"魔术数据包唤醒"（部分机型的"快速启动"会干扰）；Linux 使用 `ethtool -s <iface> wol g`；
3. **完全断电**：部分主板拔电再插上后 WOL 失效，需重新开机一次；
4. **跨网段/VLAN**：WOL 广播无法穿越路由器。把设备的"广播地址"填成目标子网广播地址（如 `192.168.2.255`），并确保路由器开启定向广播（directed broadcast）或配置了 UDP 转发（ip helper）。

## HTTP API

所有接口均需登录（Cookie 或 `Authorization: Bearer <token>`）。

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/login` | 登录，返回 JWT |
| GET | `/api/me` | 当前登录者信息 |
| GET/POST | `/api/devices` | 设备列表 / 新建设备 |
| PUT/DELETE | `/api/devices/{id}` | 更新 / 删除设备 |
| POST | `/api/devices/{id}/wake` | 唤醒单台 |
| POST | `/api/devices/wake` | 批量唤醒 `{"ids":[1,2]}` |
| POST | `/api/devices/batch` | 批量导入，重复/非法 MAC 自动跳过 |
| GET | `/api/lan/scan?cidr=x.x.x.x/24` | 扫描局域网（缺省扫描本机网段） |
| GET/POST | `/api/admins` | 管理员列表 / 新建 |
| DELETE | `/api/admins/{id}` | 删除管理员（不能删自己/最后一个） |
| PUT | `/api/admins/{id}/password` | 重置他人密码 |
| PUT | `/api/admins/me/password` | 修改自己的密码（需旧密码） |

## 从源码构建与发版

本地构建：

```bash
go build -o wol-server .                                  # 当前平台
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o wol-server .   # 交叉编译 Linux
```

本项目已配置 GitHub Actions：

- **CI**（`.github/workflows/ci.yml`）：每次推送/PR 自动 `go vet` + `go build`；
- **Release**（`.github/workflows/release.yml`）：推送 `v*` 标签自动交叉编译三个平台（linux/amd64、linux/arm64、windows/amd64），打包并生成 SHA256 校验和，自动发布到 Releases。发版只需：

```bash
git tag v1.0.1 && git push origin v1.0.1
```

## 安全建议

- 首次登录后**立即修改默认密码**；
- 通过环境变量 `WOOL_JWT_SECRET` 提供密钥时不会落盘；使用自动生成的密钥则无需管理；
- 服务仅提供 HTTP，公网环境请置于反向代理（Nginx/Caddy）之后并启用 HTTPS；
- 建议用防火墙限制管理后台的访问来源（如仅内网/VPN 可达）。

## 数据与备份

所有数据就在一个 `wol.json` 文件里，备份即拷贝：

- 每次写盘前自动将上一版备份为 `wol.json.bak`；
- 采用"临时文件 + 原子替换"写入，进程崩溃不会损坏数据；
- 文件为明文 JSON，可直接查看/手工编辑（改坏会导致启动报错，用 `.bak` 恢复即可；内含密码哈希，注意保管权限）。

## 项目结构

```
wakeonlan/
├── main.go                  # 入口（配置优先级解析、版本号注入）
├── internal/
│   ├── config/              # 环境变量配置
│   ├── model/               # 数据模型与 MAC 规范化
│   ├── db/                  # JSON 文件存储（互斥锁 + 原子写 + 自动备份）
│   ├── wol/                 # 魔术包构造与 UDP 广播发送（Windows/Unix 平台兼容）
│   ├── lan/                 # 局域网扫描（UDP 探测 + 系统 ARP 表，无需 root）
│   ├── auth/                # JWT 签发与校验
│   └── handler/             # HTTP API 与页面路由
├── web/                     # 内嵌前端（原生 JS 单页应用）
└── deploy/                  # Linux 部署文件（systemd 服务 + 安装脚本）
```

## 环境要求

- 运行：无（静态编译的单文件二进制）
- 源码编译：Go 1.26+
- 网络：服务所在机器需与被唤醒设备处于同一广播域（或可路由的定向广播）
