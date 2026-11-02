#!/usr/bin/env bash
#
# Wake-on-LAN 服务端一键部署脚本（适用于 systemd 的 Linux 发行版，如 Ubuntu/Debian/CentOS）
# 用法：
#   sudo bash install.sh
#
# 部署布局：
#   可执行文件：/usr/local/bin/wol-server
#   数据目录：  /var/local/wol        （含 wol.json、wol.json.bak）
#   服务单元：  /etc/systemd/system/wol-server.service
#
set -euo pipefail

APP_USER="wol"
BIN_SRC="$(cd "$(dirname "$0")" && pwd)/wol-server"
BIN_DST="/usr/local/bin/wol-server"
DATA_DIR="/var/local/wol"
SERVICE="wol-server"

if [[ $EUID -ne 0 ]]; then
  echo "请使用 root 运行： sudo bash $0" >&2
  exit 1
fi

if [[ ! -f "$BIN_SRC" ]]; then
  echo "未找到二进制 $BIN_SRC，请先在该目录放好 wol-server" >&2
  exit 1
fi

echo "==> 创建运行用户 $APP_USER（若已存在则跳过）"
if ! id -u "$APP_USER" >/dev/null 2>&1; then
  useradd --system --no-create-home --shell /usr/sbin/nologin "$APP_USER"
fi

echo "==> 创建数据目录 $DATA_DIR 并授权"
mkdir -p "$DATA_DIR"
chown "${APP_USER}:${APP_USER}" "$DATA_DIR"
chmod 0750 "$DATA_DIR"

echo "==> 安装可执行文件到 $BIN_DST"
install -m 0755 "$BIN_SRC" "$BIN_DST"

echo "==> 安装 systemd 服务"
install -m 0644 "$(dirname "$0")/wol-server.service" "/etc/systemd/system/${SERVICE}.service"

echo "==> 重载 systemd 并启用开机自启"
systemctl daemon-reload
systemctl enable "$SERVICE"

echo "==> 启动服务"
systemctl restart "$SERVICE"

echo
echo "部署完成。常用命令："
echo "  查看状态： systemctl status $SERVICE"
echo "  查看日志： journalctl -u $SERVICE -f"
echo "  数据文件： $DATA_DIR/wol.json（含 .bak 自动备份，可直接手工编辑）"
echo "  修改端口： 编辑 $DATA_DIR/wol.json 中的 server.listen，然后 systemctl restart $SERVICE"
echo "  访问地址： http://<服务器IP>:25352/"
echo
echo "安全说明：JWT 密钥已在数据文件的 server.jwt_secret 中自动生成并持久化，"
echo "          重启服务不会使已登录用户失效；如需指定可用环境变量 WOOL_JWT_SECRET（优先级更高）。"
