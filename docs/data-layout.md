# 本地中文语料目录

项目代码保留在 Git 工作区，中文维基语料与数据库统一放在 `E:\MathWikiData`：

```text
E:\MathWikiData\
├─ dumps\          中文维基官方原始压缩文件
├─ mysql\          MySQL 8.4 数据目录
├─ elasticsearch\  中文全文与向量索引
├─ exports\        核心条目清单、抽样报告和导出数据
└─ backups\        数据库备份
```

Docker Compose 通过 `MATHWIKI_DATA_ROOT` 绑定这些目录。以后换盘时只需修改 `.env`，不用移动代码。

核心教学条目规则位于 `config/core-curriculum.json`，数据源固定为 `zhwiki`，展示和检索文本统一为简体中文 `zh-Hans`。采集时保留原始 WikiText，额外生成简体规范化正文；英语名称只可作为搜索别名，不作为正文来源。

第一阶段先下载约 770 MB 的中文页面、分类目标、分类关系和重定向元数据进行精确盘点；盘点通过后，才下载约 3.6 GB 的中文正文 dump。

当前已生成：

- `exports/core-candidates.jsonl`：8,000 篇待正文审核候选。
- `exports/core-articles.jsonl`：5,000 篇最终简体中文核心教学条目，包含原始 WikiText、简体正文、章节、分类、来源 URL、revision ID 和许可。
- `exports/core-summary.json`：分类图盘点结果。
- `exports/content-summary.json`：正文筛选结果。
