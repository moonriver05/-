# 讲解范围、数学排版与联网搜索

## 1. 两种讲解范围

- **智能讲解（新界面默认）**：优先使用当前词条和 RAG 资料；允许基于通行定义和数学法则补全推导，并要求区分“推导补充”“背景补充”与原文支持的内容。没有有效引文时仍可给出模型讲解，但界面明确显示“暂无可核查引文”，引用置信字段为 0。
- **严格依据资料**：保留原有约束；缺少资料时说明不足，不把模型背景知识当成证据。旧客户端未指定范围时仍使用此模式。

两种模式都校验引文是否逐字存在于本次提供的资料。网址、章节和词条位置由服务端确定，不能由模型自由编造。原句匹配不等于逻辑蕴含验证；第二轮模型复核也不是人工审核或正确性保证。

主窗口和子窗口各自选择范围；提交时保存选择，不受排队期间修改控件影响。流式初稿及最终答案支持 `$...$`、`$$...$$`、`\(...\)`、`\[...\]` 数学排版；不完整公式保留文本，模型 HTML 不会作为页面代码执行。

子窗口中的学习程度、讲解范围和联网开关放在可展开的“讲解设置”中，给正文留出阅读空间。设置收起时仍显示当前范围和联网状态。

## 2. DeepSeek Search API

接口依据：[官方指南](https://api-docs.deepseek.com/zh-cn/guides/search/)、[Search 参数](https://api-docs.deepseek.com/zh-cn/api/search/)。截至 2026-10-29，官方列出的 Search 费用是 **5 元 / 千次成功返回结果的请求**，即每次约 0.005 元；空结果、失败与超时按官方当前规则处理。模型生成费用另计，价格以账户控制台和官方文档为准。

本项目调用 `POST /v1/tools/search`，每次勾选联网的提问最多请求一次，`count=3`、`need_content=true`，不自动重试、不调用另行计费的 Fetch。最多使用三份结果，每份正文节选限制 1,800 字。缺少正文时明确标为“搜索摘要”，不能当成完整网页已核查。

- 默认后端关闭，用户勾选“联网补充”后才搜索；新子窗口和清空对话后不会自动勾选。
- 搜索发送问题，必要时追加最近一条用户问题和词条标题，并截断至接口长度限制。
- 网页作为本次临时资料参与生成与引文校验；不自动写入 Chroma，不改变已有索引。
- 展示本次网页来源，带外链和摘要标识。搜索不可用或无结果时明确提示，再按选定范围使用已有资料回答。
- 搜索共用登录权限、账户并发与请求频率限制。结果不触发本机抓取；拒绝不安全链接和接口重定向，响应有大小与超时限制。

## 3. 环境变量

在仅服务端可读的 `.env` 中配置：

```dotenv
DEEPSEEK_SEARCH_ENABLED=true
DEEPSEEK_SEARCH_BASE_URL=https://api.deepseek.com
DEEPSEEK_SEARCH_API_KEY=
```

留空密钥时，Search 地址必须是官方端点，且仅当 `LLM_BASE_URL` 为空或为官方 `https://api.deepseek.com`（可带 `/v1`）才复用 `LLM_API_KEY`。模型生成经第三方代理时，需要单独填写 Search 密钥；不会把代理密钥自动发送给官方。Docker Compose 已声明这三个变量。

重启后，`/api/v1/capabilities` 的 `searchConfigured=true` 只表示配置齐全，不证明该账户已经开通接口或余额充足。实际调用失败会提示回退。所有密钥留在后端，不能配置到 `VITE_` 变量。

## 4. 现有服务器更新

这是前后端一起更新的功能，不需要重建 RAG 索引。先将 `mathwiki-search-math-update.zip` 上传至 `/opt`，执行：

```bash
(
  set -e
  unzip -o /opt/mathwiki-search-math-update.zip -d /opt
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

按上一节配置 `.env`，在宝塔 Supervisor 中重启 `mathwiki`，浏览器强制刷新。未配置联网时公式与智能讲解仍可使用，只禁用联网勾选框。

## 5. 验证范围

自动化测试使用模拟 HTTP/SSE 服务，覆盖选择传递、默认不搜索、开启后搜索、失败回退、引用来源校验、无引文标记和公式安全渲染。不会消耗真实 DeepSeek 调用额度，也不能替代实际账户开通和生产联网测试。
