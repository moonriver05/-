# 手机 H5 与搜索阅读分离

屏幕宽度不超过 850px 时，工作区改为「词条列表 / 正文阅读 / AI、批注」三个视图。默认先展示列表，点击词条后进入阅读；分类抽屉独立展开，AI 与正文不再互相覆盖。搜索框独占一行，输入字号为 16px；正文、AI 回答与触控按钮加大。支持动态视口高度及刘海屏安全区域，表格与长公式可在各自区域横向滚动。

桌面保留多栏布局。输入标题关键词、切换筛选、排序、加载更多只更新列表，不再自动选中首条结果。无结果时仍保留原正文、阅读位置和当前对话。用户主动点击另一篇词条才更换正文并清空该篇的对话。

首次进入仍预加载第一篇正文一次，手机上保持显示列表。刷新知识库保留当前所选正文；主动删除当前词条后更新选择。

## 更新服务器

这次只改前端，不需要重启 Go、MySQL 或启用 Elasticsearch、语音服务。更新包包含当前完整前端源码，也包含此前 Enter 发送、流式回答、追问和解释风格的修改。

将 `mathwiki-h5-search-update.zip` 上传到 `/opt`，在服务器终端执行：

```bash
(
  set -e
  unzip -o /opt/mathwiki-h5-search-update.zip -d /opt
  source /etc/profile.d/mathwiki-node.sh
  cd /opt/mathwiki/frontend
  pnpm install --frozen-lockfile
  NODE_OPTIONS=--max-old-space-size=384 pnpm build
)
```

包内以 `mathwiki/` 为根目录，不包含 `.env`、密钥、`node_modules` 或构建目录。Nginx 已指向 `/opt/mathwiki/frontend/dist` 时无需修改配置。编译成功后重新打开页面或强制刷新浏览器。

## 验证

- 前端类型检查通过；20 项前端测试通过，包含搜索为空、结果更换、对话保留、显式选中与手机视图切换的组件回归测试。
- 本地浏览器以模拟 API 数据检查 320、375、390、768 和 1440px 宽度：手机每次只展示一个工作区，桌面保持多栏，无页面横向溢出与运行时错误。
- 这些检查不等同于真实 iPhone/Android 的软键盘及线上网络验收，上线后仍需实机检查。
