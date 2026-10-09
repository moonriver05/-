# 流式解释与追问部署（2026-10-09）

AI 解释现在通过登录后可用的 POST `/api/v1/explanations/stream` 返回 SSE。生成的正文逐段显示；生成结束后保留正文并显示核验状态，最后替换为带引用的核验结果。仍有生成、核验两次模型请求。

同一词条可以连续追问。浏览器只发送最近六轮完成且核验成功的模型回答，服务端限制上下文长度与角色顺序。停止、失败和演示回答不进入后续模型上下文。刷新、切换词条、开始新对话或切换账户会清空对话；尚未提供跨设备的对话历史恢复。

官方流式协议参考：https://api-docs.deepseek.com/api/create-chat-completion/

感谢、夸奖、表示学会了或收尾应返回简短回应，不强行展开数学讲解。普通回应不显示核验徽标、引用折叠栏或数学解释评价按钮。Enter 发送，Ctrl + Enter 换行（Shift/Command + Enter 也支持换行）；中文输入法确认选字时不会发送。

## 更新服务器

将 `mathwiki-chat-polish-update.zip` 上传到 `/opt` 并解压覆盖源码，保留包内 `mathwiki/` 目录结构。包内没有 `.env`、密钥、数据库或本地构建产物。沿用服务器的 DeepSeek 配置，本次无需修改密钥。

这次前后端都需要在服务器构建。在宝塔终端执行：

```bash
(
set -e
source /etc/profile.d/mathwiki-go.sh
source /etc/profile.d/mathwiki-node.sh
cd /opt/mathwiki/backend
GOMAXPROCS=1 GOMEMLIMIT=256MiB CGO_ENABLED=0 go build -p 1 -trimpath -ldflags="-s -w" -o bin/mathwiki-server.new ./cmd/server
chmod 755 bin/mathwiki-server.new
cd /opt/mathwiki/frontend
pnpm install --frozen-lockfile
NODE_OPTIONS=--max-old-space-size=384 pnpm build
mv -f /opt/mathwiki/backend/bin/mathwiki-server.new /opt/mathwiki/backend/bin/mathwiki-server
)
```

仅在命令成功后，在宝塔 Supervisor 中重启 `mathwiki`，浏览器强制刷新。已有 Nginx `/api/` 反代可以继续使用，接口会返回 `X-Accel-Buffering: no` 禁用 Nginx 响应缓冲。

若浏览器仍一次性收到整段回答，检查前置代理是否忽略该响应头。可在站点现有 `location ^~ /api/` 内增加：

```nginx
proxy_buffering off;
proxy_cache off;
gzip off;
```

保留已有 `proxy_pass` 和其他设置，修改后先执行 `/www/server/nginx/sbin/nginx -t`，成功再执行 `/www/server/nginx/sbin/nginx -s reload`。

## 验收

1. 登录并发送解释问题，完整回答结束前应能看到逐段文字。
2. 核验时已有文字仍可阅读；完成后显示引用和“模型自检完成”。模型核验不等于人工审核。
3. 追问“第二步能举个例子吗？”，应针对上一轮的第二步回答。
4. 点击停止生成可继续发送；切换词条后旧回答不应串入新词条。

本地验证包括模拟模型 SSE、上下文传递、连接中断与取消、登录权限及公开条目过滤、前端 UTF-8 分段解析和类型检查。服务器真实模型及代理效果需部署后验收。
