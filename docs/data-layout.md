# 源码与数据布局

## 仓库内

- `backend/`：Go 服务、管理员工具、内容导入与索引器。
- `frontend/`：Vue 页面、组件、交互逻辑与测试。
- `datasets/public-knowledge/`：5,000 篇公开知识内容的初始化快照，包含文章、可见章节、规范化正文、来源和许可。
- `datasets/public-knowledge/manifest.json`：快照统计与 SHA-256 校验值，导入方式见 [公开快照说明](../datasets/README.md)。
- `scripts/`：启动、权限管理、数据导入、RAG 建库、正文解析和发布检查所需工具。
- `docs/submission/technical-report.docx`：维护者提供的技术文档。

## 运行数据

新安装的 Docker Compose 使用 `mysql-data`、`chroma-data` 命名卷，应用源码和运行数据分别维护。数据库账号、用户互动、上传文件、日志及实际 `.env` 保存在部署环境。

旧 Windows 安装可继续使用 `E:\MathWikiData`：

```text
E:\MathWikiData\
├─ mysql\          旧版 MySQL 数据目录
├─ elasticsearch\  可选 Elasticsearch 数据目录
├─ dumps\          自行保存的原始中文 WikiText 数据
├─ exports\        自行保存的解析结果
└─ backups\        数据库备份
```

沿用旧数据目录时使用 `infrastructure/compose-legacy-bind.yml` 并设置 `MATHWIKI_DATA_ROOT`，具体步骤见 [环境指南](environment.md)。已有数据库应继续使用与其数据目录兼容的版本。

## 内容维护

首次复现可以校验并导入仓库中的公开快照。需要更新已有原始 WikiText 的展示格式时，保留的 `scripts/render_wiki.py` 与 `backend/cmd/repair-content` 负责解析和按原章节 ID 修复内容；调用方法见 [媒体与正文维护](media-deployment.md)。

中文知识内容保留来源、修订信息和许可，展示与检索正文统一为简体中文。备份与恢复应覆盖实际业务数据库和向量数据卷，公开内容快照仅用于知识内容初始化。
