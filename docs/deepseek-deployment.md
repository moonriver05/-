# DeepSeek 接入（2026-10-08）

官方文档：https://api-docs.deepseek.com/zh-cn/

当前默认 `deepseek-flash` 对应 DeepSeek-V4.1-Flash。通过后端调用官方 Chat Completions API，关闭 thinking 模式，每次输出上限 4096 tokens。一次解释包含生成与核验两次请求，会分别消耗 API 用量。未配置密钥时仍显示演示结果；模拟测试不代表真实 API 已验收。

## 宝塔原生部署

在服务器 `/opt/mathwiki/.env` 的现有配置中修改以下三项（若启动脚本读取其他路径，以启动脚本为准）。不要覆盖数据库等其他配置，不要把真实密钥提交 GitHub 或写进前端：

```dotenv
LLM_BASE_URL=https://api.deepseek.com
LLM_MODEL=deepseek-flash
LLM_API_KEY=在服务器填写自己的密钥
ELASTICSEARCH_ENABLED=false
```

上传更新源码到 `/opt` 解压，保留 `mathwiki/` 路径。在服务器编译后端：

```bash
(
set -e
source /etc/profile.d/mathwiki-go.sh
cd /opt/mathwiki/backend
GOMAXPROCS=1 GOMEMLIMIT=256MiB CGO_ENABLED=0 go build -p 1 -trimpath -ldflags="-s -w" -o bin/mathwiki-server.new ./cmd/server
chmod 755 bin/mathwiki-server.new
mv -f bin/mathwiki-server.new bin/mathwiki-server
)
```

然后在宝塔进程守护管理器中重启 `mathwiki`。无需重新构建前端。打开一个词条，发送 AI 解释问题，确认返回解释与章节引用，不再是演示回答。错误时查看该守护进程日志；检查密钥有效性、账户余额和服务器到官方 API 的网络连接。两次远程请求需要时间；服务端单次请求超时为 90 秒。

## 搜索与内存

小内存服务器保持 `ELASTICSEARCH_ENABLED=false`，使用 MySQL 标题搜索。这个开关只改变后端查询方式，停止 ES 容器需单独执行。接入云端模型不需要在服务器加载模型权重。

新增讲解范围、公式排版和 Search API 需同时更新前后端，步骤见 [讲解与联网更新](answer-modes-search.md)。仅修改现有模型环境变量时无需重新构建前端。
