# 批注高亮更新

## 使用方式

1. 在正文中选中 2—240 字，点击浮动工具栏中的“批注”。输入批注时，选中的文字保留临时高亮。
2. 发布后，对应文字显示黄色高亮。普通用户提交的批注仍沿用现有审核规则；只有接口允许当前用户查看的批注会出现在正文中。
3. 点击批注卡片中的引用或“定位原文”，可滚动到原文，并加深对应高亮。手机上会自动切回“正文阅读”。
4. 点击正文高亮可查看相应批注；同一位置有多条批注时，重复点击可依次切换。
5. 刷新页面后保留高亮，删除批注后相应标记消失。

## 定位与兼容

新批注保存章节编号、选中文字、前后文与 UTF-16 字符偏移，支持跨加粗、链接等格式的文字范围。重叠批注共享标记，不修改原始正文或存储 HTML。

旧批注只有在章节和引用能够唯一匹配时才高亮。若原文已改动或旧引用重复，仍保留批注内容，点击时提示无法准确定位。公式内部暂不支持文字高亮；请选择公式旁的解释文字。

使用现有批注表字段，无新增环境变量、服务或索引任务。已有 RAG 索引无需重建。

## 宝塔服务器更新

将 `mathwiki-annotation-highlight-update.zip` 上传到 `/opt`，然后运行下面的命令。压缩包包含完整前后端源码，根目录是 `mathwiki/`，不含实际 `.env`、数据库或依赖目录。

```bash
(
  set -e
  unzip -o /opt/mathwiki-annotation-highlight-update.zip -d /opt
  source /etc/profile.d/mathwiki-go.sh
  source /etc/profile.d/mathwiki-node.sh

  cd /opt/mathwiki/backend
  GOPROXY=https://goproxy.cn,direct GOMAXPROCS=1 GOMEMLIMIT=256MiB CGO_ENABLED=0 \
    go build -p 1 -trimpath -ldflags="-s -w" \
    -o bin/mathwiki-server.new ./cmd/server
  chmod 755 bin/mathwiki-server.new

  cd /opt/mathwiki/frontend
  pnpm install --frozen-lockfile
  NODE_OPTIONS=--max-old-space-size=384 pnpm build

  mv -f /opt/mathwiki/backend/bin/mathwiki-server.new \
    /opt/mathwiki/backend/bin/mathwiki-server
)
```

命令成功结束后，在宝塔进程守护中重启 `mathwiki`，浏览器强制刷新；手机可关闭该网页后重新打开。必须同时更新后端和前端，才能在刷新后保留重复句子的准确位置。

## 验证

前端 `pnpm test` 覆盖重复句子、跨格式选择、重叠、删除、公式保护、恶意 HTML 文本、发布后重新加载及手机定位；`pnpm build` 验证生产构建。后端 `go test ./...` 和 `go vet ./...` 检查批注锚点返回及现有功能。另在 1440px 桌面和 390px 手机视口验证发布、刷新、双向定位与明暗主题。
