# 完整环境与安装指南

## 1. 依赖、版本与运行方式

| 部件 | 依赖依据 | 用途 |
| --- | --- | --- |
| 后端 | Go >= 1.23，go.mod / go.sum | Gin、GORM、MySQL / SQLite、云适配器 |
| 前端 | Node 22.18+，pnpm 10.17.1，package.json / pnpm-lock.yaml | Vue、TypeScript、Vite、KaTeX、DOMPurify |
| 数据库 | Compose MySQL 8.4；现生产实例为 MySQL 9.0.1 | 新部署优先遵循 Compose，勿直接降级现有数据目录 |
| RAG | Compose 固定 Chroma 镜像摘要；Docker + Compose v2 | 可选，仅启用 rag profile 才启动 |
| 正文解析 | Python 3.10+，scripts/requirements.txt | 仅重新解析 WikiText 时需要 |

README 给出不依赖云密钥的 SQLite 启动；需要真实 AI 对话时还需模型密钥、登录账户及 SMTP 验证，不能靠演示模式证明已接入真实模型。

保留的辅助工具用于 Windows 启动与管理员授权、公开快照导入、RAG 索引维护、WikiText 正文解析及发布检查。环境文件和依赖锁文件与对应源码一并维护。

## 2. 环境文件加载规则

复制根目录 `.env.example` 为 `.env`，只在本机编辑；真实文件已忽略。

- Windows `scripts/dev.ps1` 和 `scripts/run-rag-indexer.py` 读取根目录 .env。
- Docker Compose 自动读取 .env 做变量替换；只向容器传递 compose 中声明的项目。
- `go run` / 编译后二进制只读取进程环境，不自动读取 .env。用终端导出，或由 Supervisor 的现有启动包装器加载。
- Linux 手动示例：`export DATABASE_DRIVER=mysql`；`export MYSQL_DSN='用户:密码@tcp(127.0.0.1:3306)/mathwiki?charset=utf8mb4&parseTime=True&loc=Local'`。DSN 必须加引号，不要直接把含特殊符号的整份 .env 当成 shell 脚本执行。
- 所有密钥只能放后端；VITE_ 变量会进入浏览器构建产物。

## 3. 核心变量

| 变量 | 示例 / 默认 | 说明 |
| --- | --- | --- |
| APP_PORT | 8080 | API 监听端口 |
| DATABASE_DRIVER | mysql / sqlite | 正式使用 MySQL；SQLite 用于开发和测试 |
| MYSQL_DSN | 见模板 | 原生进程连接串，含数据库名和 utf8mb4 |
| SQLITE_PATH | mathwiki-dev.db | 开发文件路径，勿提交数据库 |
| DEMO_SEED | true | 空库演示数据；导入公开快照时设 false |
| AUTH_COOKIE_SECURE | false | 仅本地 HTTP；正式 HTTPS 应设 true |
| MYSQL_PASSWORD | 本地示例 | Compose 创建普通数据库账号；上线必须更换 |
| MYSQL_ROOT_PASSWORD | 本地示例 | Compose 初始化 root 密码 |
| MATHWIKI_DATA_ROOT | E:/MathWikiData | 旧 bind 挂载及语料工具使用；新 Compose 数据库使用命名卷 |

不要在已有数据目录上通过改变量“重置密码”，初始化变量仅首次生效。MySQL 密码若包含影响 DSN 解析的分隔符，应遵循驱动格式处理；优先为评审专用实例生成不含 DSN 分隔符的独立强密码。

## 4. AI、RAG 与使用限额

| 变量 | 默认 / 示例 | 说明 |
| --- | --- | --- |
| LLM_BASE_URL | https://api.deepseek.com | 兼容 chat/completions 的基地址 |
| LLM_API_KEY | 空 | 无密钥只返回明确标记的演示解释 |
| LLM_MODEL | deepseek-flash | 本项目当前配置值；可用模型以开通账户为准，不宣称永远最新 |
| DEEPSEEK_SEARCH_ENABLED | false | 可选联网搜索；后端开启且用户勾选后才调用 |
| DEEPSEEK_SEARCH_BASE_URL | https://api.deepseek.com | Search API 地址；代理服务必须使用独立配置的密钥 |
| DEEPSEEK_SEARCH_API_KEY | 空 | 可留空复用官方 DeepSeek 的 LLM_API_KEY；第三方模型服务地址不会自动复用密钥 |
| AI_MAX_CONCURRENT | 2 | 单 API 进程全局最多并行回答，允许 1-8 |
| AI_REQUESTS_PER_MINUTE | 6 | 每账户一分钟窗口最多准入数，允许 1-60 |
| RAG_ENABLED | false | 显式开启向量检索 |
| ARK_API_KEY | 空 | 方舟 embedding 密钥，与 DeepSeek 密钥不同 |
| ARK_BASE_URL | https://ark.cn-beijing.volces.com/api/v3 | embedding 基地址 |
| ARK_EMBEDDING_MODEL | doubao-embedding-vision-251215 | 采用纯文本输入、固定 1024 维 |
| CHROMA_URL | http://127.0.0.1:8000 | 原生 API / 索引器连接地址 |
| CHROMA_DOCKER_URL | http://chroma:8000 | Compose 内 API 专用地址 |
| CHROMA_COLLECTION | mathwiki_rag_1024_v1 | 模型、维度、格式需要一致 |
| CHROMA_IMAGE | 见 .env.example 的摘要 | Compose 固定镜像，不自动跟随 latest |

同账户最多一条生成中的回答；同步与流式端点共享配额。忙碌时在模型调用前返回 429 或 503，并提供 Retry-After。取消、错误和异常会释放并发位置；已准入的错误请求仍计入窗口。配额在进程内，重启会重置，多实例须改共享限额，不是计费账本。

先通过 `/api/v1/capabilities` 看是否配置，不能用 ragConfigured=true 证明索引可访问。管理员 `/api/v1/admin/rag/status` 才检查集合可读性与条数。首次建库先运行 `-plan`；改模型或维度需新集合并重建，不能混写旧向量。

## 5. 邮件、头像和可选服务

SMTP_HOST、SMTP_PORT（465）、SMTP_TLS_MODE（tls / starttls）、SMTP_USERNAME、SMTP_PASSWORD、SMTP_FROM：注册及密码重置验证码。没有 SMTP 不跳过验证，不返回演示验证码。先注册再由管理员工具赋权，详见 accounts.md。

COS_BUCKET、COS_REGION、COS_SECRET_ID、COS_SECRET_KEY：腾讯 COS 头像；COS_SESSION_TOKEN 仅临时凭据时用。未配置不影响知识阅读。

ELASTICSEARCH_ENABLED=false、ELASTICSEARCH_URL、ELASTICSEARCH_INDEX：可选 ES；默认不启用，现生产标题搜索使用 SQL。

STT_PROVIDER=doubao、DOUBAO_SPEECH_API_KEY：可选语音适配，生产不配置。浏览器麦克风需要安全上下文；此次提交不演示语音。TENCENT_SECRET_ID / KEY、VOLC_ACCESS_KEY / SECRET_KEY 为历史预留项，当前语音适配不读取，留空即可。

VITE_WIKI_IMAGE_MODE（external / mirror-only / off）、VITE_MEDIA_MANIFEST_URL、VITE_MEDIA_ALLOWED_ORIGINS：浏览器媒体策略，修改需重建前端，见 media-deployment.md。

TEST_MYSQL_DSN、CHROMA_TEST_URL：仅对隔离测试实例设置，绝不指向生产数据；CI 默认留空，真实集成测试跳过。

## 6. 公开内容导入与管理员

先设 `DEMO_SEED=false`，停止目标 API 及其他内容写入，校验并导入空内容库：

```bash
go -C backend run ./cmd/knowledge -mode verify -dir ../datasets/public-knowledge
# MYSQL_DSN 已导出到当前进程环境后；此导入工具仅面向 MySQL：
go -C backend run ./cmd/knowledge -mode import -dir ../datasets/public-knowledge
```

不得向已有内容库重复导入；已有内容会拒绝，绝不为评审清空生产。该快照只含公开内容，不创建账户。Windows 可使用 `scripts/public-knowledge.ps1` 加载 .env。

用配置好的邮件服务注册评审账户，再执行 `scripts/set-user-role.ps1 -Username 账号 -Role admin`。不向 Public 仓库提交可登录密码；评审仅需代码可访问，若另需网站账户，通过私下渠道提供。

## 7. 网络、数据与排错

Compose 默认 web=5173，API / MySQL / Chroma 只映射宿主 localhost 的 8080 / 3306 / 8000；可选 ES 为 localhost:9200。生产通过 Nginx 同源访问 API，不对外开放向量数据库。

新安装持久卷为 mysql-data、chroma-data；旧 bind 用户采用 infrastructure/compose-legacy-bind.yml，不直接更换数据位置。现生产为手动 Docker 部署，与 Compose 是两条独立路线。备份数据库及向量卷时停止相应写入、使用数据库一致性备份并验证恢复，不能把公开快照当完整业务备份。

AI 慢：分别确认检索、初稿、自检阶段，不能只看总时长。RAG 回退：检查容器、密钥、模型 / 维度 / 格式和相关性，不能无条件调高距离阈值。`c.index undefined`：覆盖了不匹配的源码，使用完整更新包，不删除索引。浏览器无流式：检查代理是否遵守 X-Accel-Buffering:no。
