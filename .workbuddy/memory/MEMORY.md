# Wake-on-LAN 项目备忘（长期沉淀）

## 工作流约定
- **每轮任务完成后，git commit 并 push 到 origin**（用户 2026-11-03 明确要求）。origin 为双 push：codeup.aliyun.com:01/other/wakeonlan + github.com:zc0350/wakeonlan，会同时推送。
- 提交信息：简洁中文，说明改动要点。

## 项目速览
- Go 语言 WOL 管理平台：Web 管理、局域网扫描、单台/批量唤醒、多管理员。单二进制（go:embed 内嵌前端），数据为单 JSON 文件。
- Go 安装在 C:/Go，命令前需 `export PATH=$PATH:/c/Go/bin`。
- 本机测试注意：curl 访问 127.0.0.1 必须加 `--noproxy '*'`（否则走本地代理失败）。
- 配置全在 wol.json 的 server 段：listen（默认 **:25352**，用户指定，不要 8080）、jwt_secret（缺失自动生成并持久化）、wake_attempts=3、wake_interval_ms=100；缺省/非法值启动自愈；环境变量（WOOL_*）优先级高于 JSON。
- Linux 部署布局：二进制 /usr/local/bin/wol-server，数据 /var/local/wol/wol.json，systemd 服务 wol-server.service，deploy/install.sh 一键安装。
- 发版：`git tag vX.Y.Z && git push origin vX.Y.Z` → GitHub Actions 自动构建 linux/amd64+arm64、windows/amd64 并发布 Release。
