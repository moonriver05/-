# 评审版本更新与上线

本轮新增 AI 准入限制（默认全局 2、每账户同时 1、每分钟 6）并共用在同步与流式接口；过载在调用云端前拒绝，含 Retry-After。取消、错误和 panic 都释放位置，进程内记录有过期回收和容量上限。新增测试覆盖跨接口配额、并发拒绝、窗口恢复及异常释放。

未配置语音时隐藏麦克风入口；ES 默认关闭，避免未配置本地启动仍访问 ES。Compose 分离原生 / 容器 Chroma 地址，数据库密码走环境配置，API 与数据库端口只绑定 localhost。

完整更新包 `mathwiki-review-complete.zip` 含 backend、frontend 源码、锁文件和文档，不含 .env、数据库、node_modules、dist 或 bin。上传到 /opt 后解压到 /opt，重编译后端和前端，再重启 Supervisor 的 mathwiki；保持原 .env，不覆盖已有账号密码及数据目录。已有 48,560 段无需重建。

```bash
(
set -e
unzip -o /opt/mathwiki-review-complete.zip -d /opt
source /etc/profile.d/mathwiki-go.sh
source /etc/profile.d/mathwiki-node.sh
cd /opt/mathwiki/backend
GOPROXY=https://goproxy.cn,direct GOMAXPROCS=1 GOMEMLIMIT=256MiB CGO_ENABLED=0 \
go build -p 1 -trimpath -ldflags="-s -w" -o bin/mathwiki-server.new ./cmd/server
chmod 755 bin/mathwiki-server.new
cd /opt/mathwiki/frontend
pnpm install --frozen-lockfile
NODE_OPTIONS=--max-old-space-size=384 pnpm build
mv -f /opt/mathwiki/backend/bin/mathwiki-server.new /opt/mathwiki/backend/bin/mathwiki-server
)
```

编译成功后在 Supervisor 重启 mathwiki，刷新网页。如果 Nginx 实际使用其他静态目录，按原部署路径发布 dist，不改服务器既有目录指向。上述前端构建直接生成当前 dist，部署前应备份旧 dist 以便回滚。

Chroma 当初手动创建时 restart=no；确认服务可用后可执行 `docker update --restart unless-stopped mathwiki-chroma`，只调整重启策略，不重建容器、不改卷和内存。**不要在现生产主机直接执行新 Compose 来接管现有 MySQL / Chroma。** 旧 Compose 存储兼容见 environment.md。

验收：现有登录、搜索、原文引用仍可用；两处 AI 入口共享保护；同账户生成中再次发起请求返回 429，正常完成后可继续；未配置语音时无麦克风。并发测试使用独立测试账户，勿批量请求真实云 API 产生额外费用。
