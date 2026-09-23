# 公开中文知识库

`public-knowledge/` 是可分发的初始化内容快照，不是完整数据库备份。每个 JSONL 文件不超过 20 MiB，一行包含一篇完整文章和当前可见章节；`manifest.json` 记录数量与 SHA-256 校验值。

当前快照导出于 2026-09-23：**5,000 篇文章、48,845 个可见章节、25 个条目主分类、8 个分片，共约 156.46 MiB**。不含库中 1 篇待审核投稿及 4,216 个隐藏旧章节。已通过全部分片校验和临时数据库导入/再导出一致性测试。

## 公开范围与隐私

- 只包含已发布、未删除、非待审/驳回的中文维基百科导入条目，及其可见章节。
- 保留标题、分类、标签、摘要、正文、HTML、公式、图片链接、原文链接、页面/修订编号、许可证和内容解析提示。
- 不包含本地账号、邮箱、密码哈希、头像、验证码、登录会话、批注、评论、反馈、收藏、阅读历史或 AI 对话。
- 不包含待审社区投稿、隐藏旧章节、本地行编号、内部时间戳及原始 WikiText。导入后从可见章节重建检索纯文本；需要重新解析 WikiText 时应另外从上游获取对应修订。
- 图片仅保留原有链接，不包含图片文件，不保证外部链接永久可用。媒体文件的许可证以原始文件页为准。

## 来源与许可

文章内容来自中文维基百科，数据包内标注为 **CC BY-SA 4.0**。请保留每篇文章的 `sourceUrl`、`pageId`、`revisionId`、`sourceTitle` 和 `license` 字段。作者名单和编辑历史可从原文页面的“查看历史”访问，对应修订可通过 `https://zh.wikipedia.org/w/index.php?oldid=修订编号` 查看。

本项目对内容进行了简体转换、数学教学分类、章节结构化及 HTML 渲染，内容未经逐篇人工核验；`machine_selected` 不等于人工审核通过。重新发布改编内容时保留来源、修改说明与相同许可。许可证全文：[CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/)。数据许可不自动适用于项目代码或第三方图片。

## 校验和导入

在项目根目录执行（已安装 Go）：

```powershell
# 仅校验，不连接数据库。
go -C backend run ./cmd/knowledge -mode verify -dir ../datasets/public-knowledge

# 从本地 .env 读取 MYSQL_DSN；先将其配置为需要初始化的 MySQL 数据库。
# 导入目标的 articles、sections 必须为空。不会覆盖现有知识库。
.\scripts\public-knowledge.ps1 -Mode import
```

数据库须预先建立。导入只创建/写入内容表（articles、sections、categories），不会创建用户或管理员。请停止目标后端及其他内容写入进程后导入；首次启动后端时设 `DEMO_SEED=false`，避免填充演示内容。若使用 Elasticsearch，请按项目现有索引流程重新建立索引。数据库备份不能用本快照替代。

导入在事务中进行，任何分片校验或数据写入失败都会回滚内容写入；建表操作不属于可回滚的内容事务。目标已有条目时拒绝导入，不会清空数据库。

## 更新快照

```powershell
# 只读源数据库，导出到一个尚不存在的目录。不会覆盖已有快照。
.\scripts\public-knowledge.ps1 -Mode export -Directory datasets/public-knowledge-next
```

导出完成后必须重新运行密钥扫描并人工审核，再发布。不要将整库 SQL 或本地 `.env` 加入 Git。频繁更新大数据会增加 Git 历史体积，后续批量快照建议用 Release 分发。
