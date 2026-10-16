# 豆包 Embedding + Chroma RAG

当前实现：用户问题 → 豆包 `doubao-embedding-vision-251215`（纯文本、1024 维）→ Chroma 当前词条优先和全库检索 → MySQL 读取有效公开片段 → DeepSeek 流式回答与第二轮自检 → 服务端校验原文引用。

不运行本地 embedding/LLM 模型，不重新启用 Elasticsearch。普通标题搜索保持原行为。Chroma 只存向量与章节 ID，不存全文；MySQL 是正文和公开状态的来源。

## 当前试运行限制

- 每段最多 800 字、重叠 100 字；每章只索引前 12000 字。隐藏章节、模板附录、未发布、待审、拒绝和已删除条目不进入索引。
- 首次默认处理前 1000 个片段，不等于整个 5000 篇知识库已覆盖。`-coverage` 可为每篇有效词条补一个片段，`-all` 补齐所有可索引片段。默认集合预算为 10000 条，可经内存评估后用 `-max-records` 显式调整（最高 100000），容器内存上限不会自动增加。
- `-limit 2000` 表示扫描前 2000 段，重复的片段不会再调用 embedding；修改后的片段生成新 ID。旧 ID 暂不自动删除，会占据容量，但查询时会检查内容摘要，旧文本不会用于回答。
- RAG 检索最多并发 2 个，整体检索超时 10 秒；最终最多 5 段、合计最多 4000 字证据。
- 回退只在当前条目中选证据，最多读取前 60 个有效章节，每章前 12000 字。证据不足时应明确说明，不表示模型具备整个词条的全部内容。
- 模型引用必须逐字出现在证据片段中。无法匹配的引用丢弃，没有任何有效引用时不报告完成。这是引用一致性检查，不是数学正确性的人工证明。
- Chroma 停止、超时、无匹配、向量规格不符时，回退当前词条，并在回答页面显示回退状态。页面可点击「查看原文」打开对应章节；跳到另一篇词条时开始该词条的新对话。

## 1. 服务器 .env

在服务器 `/opt/mathwiki/.env` 内追加或更新以下变量，**真实密钥仅填在服务器**：

```dotenv
RAG_ENABLED=true
ARK_API_KEY=你的火山方舟API密钥
ARK_BASE_URL=https://ark.cn-beijing.volces.com/api/v3
ARK_EMBEDDING_MODEL=doubao-embedding-vision-251215
CHROMA_URL=http://127.0.0.1:8000
CHROMA_COLLECTION=mathwiki_rag_1024_v1
```

保留现有 `LLM_*`、MySQL、邮件和 COS 配置。方舟 API Key 与豆包语音 API Key 不同。先在方舟开通该模型；如账户要求接入点，可将 `ARK_EMBEDDING_MODEL` 填为对应 `ep-...`，以后不要换模型却复用旧集合。

1024 维是固定协议，改模型/维度/分段规格必须新建集合并重建。向量服务按实际请求计费：首次建库、内容更新和用户提问都会调用，未修改内容不会在每次提问时重复向量化。

## 2. 更新源码、编译

将 `mathwiki-rag-chroma-update.zip` 上传至 `/opt`。包内根目录是 `mathwiki/`，包含完整前后端源码和此前 H5 修改，不包含 `.env` 或二进制。执行：

```bash
(
  set -e
  unzip -o /opt/mathwiki-rag-chroma-update.zip -d /opt
  source /etc/profile.d/mathwiki-go.sh
  source /etc/profile.d/mathwiki-node.sh
  cd /opt/mathwiki/backend
  GOMAXPROCS=1 GOMEMLIMIT=256MiB CGO_ENABLED=0 go build -p 1 -trimpath -ldflags="-s -w" -o bin/mathwiki-server.new ./cmd/server
  GOMAXPROCS=1 GOMEMLIMIT=256MiB CGO_ENABLED=0 go build -p 1 -trimpath -ldflags="-s -w" -o bin/mathwiki-rag-indexer ./cmd/rag-indexer
  chmod 755 bin/mathwiki-server.new bin/mathwiki-rag-indexer
  cd /opt/mathwiki/frontend
  pnpm install --frozen-lockfile
  NODE_OPTIONS=--max-old-space-size=384 pnpm build
  mv /opt/mathwiki/backend/bin/mathwiki-server.new /opt/mathwiki/backend/bin/mathwiki-server
)
```

沿用宝塔 Supervisor 启动脚本，它必须将 `.env` 所有变量传给程序。此时暂不重启，先建库。

## 3. 小批量建库

已启动的 `mathwiki-chroma` 使用 `/data` 持久卷及仅本地开放的 8000 端口，继续使用即可，不要再创建同名容器。

```bash
curl --noproxy '*' -sS --max-time 10 http://127.0.0.1:8000/api/v2/heartbeat
GOMAXPROCS=1 GOMEMLIMIT=128MiB python3 /opt/mathwiki/scripts/run-rag-indexer.py -limit 1000
docker stats --no-stream mathwiki-chroma
free -h
```

脚本兼容 Python 3.6，读取服务器 `.env` 后直接执行 Go 索引程序，不需要安装 Python embedding 模型。串行向量请求，每新增一条后至少等待 200ms；1000 条可能需要数分钟，不是后台持续驻留服务。可按 Ctrl+C 停止，重新运行会跳过成功片段，不清空任何数据库。

观察建库和实际问答占用，确认没有 OOM/明显交换抖动，再逐步用 `-limit 2000` 扩大范围。不要只凭空库 32MiB 就认定全部知识库够用。

### 只补建一篇词条

前 1000 段按章节 ID 排序，只覆盖开头的一部分内容。页面上能读到某篇词条不代表它已经有向量。接口可用但候选匹配距离均大于 0.65 时，不应为了显示命中而直接放宽门槛。

支持 `-article-id` 按词条编号补建。例如当前线上「前向错误更正」编号为 5105，可执行：

```bash
GOMAXPROCS=1 GOMEMLIMIT=128MiB python3 /opt/mathwiki/scripts/run-rag-indexer.py -article-id 5105 -limit 1000
```

这里 `-limit 1000` 是这篇词条的处理上限，不会硬凑 1000 段。仅追加该篇真实存在的有效片段，不删除已有 1000 段；重复执行仍会跳过原有向量。词条不存在、未公开或无有效章节会返回明确错误。建库后网站会直接检索新增向量，无需重启后端。

若正在使用最初的 RAG 更新包，先将 `mathwiki-rag-targeted-update.zip` 上传至 `/opt` 并更新建库工具：

```bash
(
  set -e
  unzip -o /opt/mathwiki-rag-targeted-update.zip -d /opt
  source /etc/profile.d/mathwiki-go.sh
  cd /opt/mathwiki/backend
  GOPROXY=https://goproxy.cn,direct GOMAXPROCS=1 GOMEMLIMIT=256MiB CGO_ENABLED=0 \
    go build -v -p 1 -trimpath -ldflags="-s -w" -o bin/mathwiki-rag-indexer.new ./cmd/rag-indexer
  chmod 755 bin/mathwiki-rag-indexer.new
  mv -f bin/mathwiki-rag-indexer.new bin/mathwiki-rag-indexer
  echo "建库工具更新完成"
)
```

此补丁只改建库程序，不需要编译前端或重启 Supervisor。索引后重新提问验证是否命中；如仍无命中，继续检查匹配距离及原文有效性，不直接放宽 0.65 门槛。

## 4. 重启与验证

在宝塔 Supervisor 中重启 `mathwiki`，浏览器强制刷新。测试与已索引内容相关的问题：页面应显示「正在检索相关知识」以及「知识检索 · N 段证据」，引用可打开原文。

```bash
docker inspect --format 'status={{.State.Status}} OOMKilled={{.State.OOMKilled}} exit={{.State.ExitCode}}' mathwiki-chroma
curl --noproxy '*' -sS --max-time 10 http://127.0.0.1:8080/api/v1/capabilities
```

`ragConfigured=true` 仅表示开关与密钥已配置，不代表检索成功。管理员登录后可读取 `/api/v1/admin/rag/status` 查看 `ready` 和 `chunks`；普通用户无法访问该状态接口。未覆盖的词条可能回退，属于试运行阶段的正常情况。

如要停用，只需将 `.env` 的 `RAG_ENABLED=false` 后重启后端。向量数据保留，网站继续使用当前词条证据。

## 本地验证与实际限制

后端模拟服务覆盖 embedding 协议/1024 维归一化、建库幂等、公开状态与过期内容过滤、取消、错误回退和并发限制。使用与你服务器相同摘要的真实 Chroma 镜像验证了建集合、写入、重复建库、count 和跨词条查询。前端 21 项回归测试覆盖检索阶段、回退状态、章节链接与原有 H5/对话功能，类型检查通过。

临时 Docker 容器在 384MiB/1 核限制下写入 1000 条 1024 维合成测试向量，并连续查询 20 次，通过后 Docker 内存快照约 42.18MiB。该数值是测试结束后的本地快照，不是全过程峰值，也不代表真实服务器的全部知识库。临时测试容器已清理。

方舟调用尚未用真实账户密钥验收；真实服务器 1000 段建库和峰值内存仍需上述步骤确认。

官方资料：[方舟多模态向量化 API](https://docs.volcengine.com/docs/ark/multimodal-vectorization-api?lang=zh)、[Chroma Docker 与持久化](https://cookbook.chromadb.dev/running/running-chroma/)。
