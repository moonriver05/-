# 多窗口 AI 问答与共享上下文

## 使用

在右侧“AI 解释”中点击“新建子窗口”，最多可同时保留主窗口和三个子窗口。每个窗口有独立的输入、学习程度、解释方式和问答展示；所有窗口共用当前词条与最近已完成的模型对话。

- 桌面上可以拖动标题栏、调整窗口大小、收起和恢复。收起时继续回答，并保留未发送草稿。
- 手机上每次展示一个子窗口，通过窗口标题处的选择框或主面板的窗口按钮切换。
- “查看共享记录”列出当前可供所有窗口追问的已完成记录，标明来源窗口。
- 关闭窗口会停止该窗口尚未完成的问题；已经完成的问答仍可被其他窗口追问。
- “开始新对话”清空全部窗口的共享记录；多个窗口打开时会先确认。切换词条、退出账号或会话失效也会清空记录和草稿。

例如，在主窗口问“为什么要求边权非负”，再到子窗口问“用生活例子解释刚才的答案”，子窗口会使用前一轮已经完成的回答作为对话历史。另一个窗口可以继续追问该生活例子。

## 顺序与边界

同一页面的请求按提交顺序处理，每个窗口最多有一个待处理问题。排队中的问题在真正开始时读取最新已完成的共享对话，因此能接着前面的窗口回答继续问。取消排队不会调用模型；生成中取消会中止请求。已有服务器并发及频率限制继续生效。

收到 429（限流）或 503（服务繁忙）时，失败问题保留供用户修改后重发，其他排队问题按 `Retry-After` 等待后再发送，界面显示等待状态。未给出有效等待时间时使用短暂等待；不会自动重发失败问题，因此不会因重试重复触发搜索或生成。取消、清空和切换词条会移除旧队列，已知的账户等待时间仍生效。

共享历史使用现有接口的上限：最近最多六轮已完成的模型问答、总计不超过 32,000 个字符；失败、取消、未完成和演示回答不进入模型历史。页面最多保留 80 轮记录，每个窗口展示其最近 20 轮。

这里的窗口属于当前网页内的同一个会话。刷新页面后清空；不同浏览器页面、设备和用户之间不共享。当前词条和检索证据仍由原有后端 RAG 流程提供。

初稿生成和第二轮模型复核都获得本轮请求的同一份历史。复核使用历史理解“第二步”“刚才那个例子”等指代和用户纠正；历史不是事实证据，仍以当前问题、当前学习程度和资料边界为准。复核增加了已有历史的输入 token，不增加模型请求次数，也不新增服务或向量索引。

## 实现

- `frontend/src/sharedChats.ts`：页面共享记录、独立窗口状态、顺序调度、取消和上下文边界。
- `frontend/src/components/AiChatWindow.vue`：子窗口、输入、流式回答、引用导航及拖动。
- `frontend/src/chatWindows.css`：桌面窗口与手机切换布局。

复用 `POST /api/v1/explanations/stream` 和现有 `history` 字段，无新增数据库、服务、环境变量或模型依赖。

## 更新现有服务器

2026-10-31 的追问复核与队列修复需要同时更新前后端。将 `mathwiki-followup-queue-update.zip` 上传到 `/opt` 后运行（包中也包含之前的公式和可选联网搜索功能）：

```bash
(
  set -e
  unzip -o /opt/mathwiki-followup-queue-update.zip -d /opt
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

构建成功后在 Supervisor 中重启 `mathwiki`，浏览器强制刷新。RAG 索引无需重建。新部署仍按 [环境指南](environment.md) 配置模型和流式代理；启用联网另见 [讲解范围与联网搜索](answer-modes-search.md)。

## 检查

`pnpm test` 覆盖跨窗口上下文、按顺序处理、限流等待及不自动重试、取消、窗口关闭、旧请求迟到、历史与内存边界，以及实际 Vue 窗口的收起、恢复、独立草稿和全局清空。`pnpm build` 检查类型和生产构建。后端测试使用模拟模型服务，检查两轮请求中的历史及证据边界；不消耗真实模型额度。
