# 全库索引与内存预估

当前约 5000 篇词条不等于 5000 条向量。每个正文片段最多 800 字，重叠 100 字，一篇长词条会生成很多向量。1024 维 float32 的原始数据占用：5000 条约 19.5MiB，50000 条约 195.3MiB；这些不是 Chroma 总内存，还需要索引结构、查询和服务开销。

本次更新只改建库工具，保留原向量、方舟配置和网站。默认先对所有有效词条各建一个检索入口，再根据真实规模决定正文分批扩展。

## 更新工具

上传 `mathwiki-rag-full-index-update.zip` 到服务器 `/opt`，执行：

```bash
(
  set -e
  unzip -o /opt/mathwiki-rag-full-index-update.zip -d /opt
  source /etc/profile.d/mathwiki-go.sh
  cd /opt/mathwiki/backend
  GOPROXY=https://goproxy.cn,direct GOMAXPROCS=1 GOMEMLIMIT=256MiB CGO_ENABLED=0 \
    go build -v -p 1 -trimpath -ldflags="-s -w" -o bin/mathwiki-rag-indexer.new ./cmd/rag-indexer
  chmod 755 bin/mathwiki-rag-indexer.new
  mv -f bin/mathwiki-rag-indexer.new bin/mathwiki-rag-indexer
  echo "全库建库工具已更新"
)
```

不需要重新编译前端或重启 Supervisor。

也可解压后用 `bash /opt/mathwiki/scripts/build-rag-indexer.sh` 完成相同的工具编译步骤。若文本编辑器把脚本转成 Windows 换行，先执行 `sed -i 's/\r$//' /opt/mathwiki/scripts/build-rag-indexer.sh` 再运行。

## 先统计，再覆盖全部词条

```bash
GOMAXPROCS=1 GOMEMLIMIT=128MiB python3 /opt/mathwiki/scripts/run-rag-indexer.py -plan
```

只扫描 MySQL，输出可索引词条、章节、正文片段数量及向量原始大小，不调用 embedding，不写入或删除索引。每页只取 40 章、每章最多 12000 字，不将全部正文载入 Go 内存。

为每篇词条添加首个有效章节的第一段（初步全词条覆盖）：

```bash
GOMAXPROCS=1 GOMEMLIMIT=128MiB python3 /opt/mathwiki/scripts/run-rag-indexer.py -coverage
docker stats --no-stream mathwiki-chroma
free -h
```

这会调用 embedding，仅为缺少的片段付费；原来的 1000 段和定向补建数据保留。重复执行跳过未变化片段。约 5000 篇通常需要新增数千条向量，可能运行几十分钟，实际时长取决于 API 延迟和限流。Ctrl+C 可停止，再执行会跳过已成功内容。未发布、待审、拒绝、删除、隐藏、空白和模板附录不加入索引。

**全词条覆盖不等于完整正文覆盖。** 每篇入口更适合发现相关词条，细节问答还需要更多章节证据。

## 全部可索引正文

```bash
GOMAXPROCS=1 GOMEMLIMIT=128MiB python3 /opt/mathwiki/scripts/run-rag-indexer.py -all
```

会先做规模预估。超过默认 10000 条集合预算则在调用向量 API 前停止，并报告所需数量，不默默只索引前 10000 段然后声称完成。预算也包括尚未清理的旧向量，写入时再次校验，超限停止时已成功写入的数据保留。

若统计显示全文超过预算，先结合 `docker stats` 的导入/查询占用、OOM 状态和 `free -h` 评估，再显式设置 `-max-records N`，不要只为通过检查设置为最大值。此参数控制记录数量，不会预留内存，也不会提高 Docker 的 384MiB 上限。

当前分段协议仍每章最多前 12000 字；`-plan` 会报告达到此上限的章节数量。因此这里的「全部」是全部**可索引**片段，不包含被截取的长章节尾部，也不包含排除的内容。

`-plan`、`-coverage`、`-all` 与 `-article-id` 互斥。原有 `-limit` 前缀模式和定向补建仍可使用，不影响普通标题搜索。
