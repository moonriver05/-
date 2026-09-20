# 图片存储、流量与部署

## 当前实现（2026-09-17）

- 正文和结构化 HTML 存在 MySQL，浏览器从 Go API 获取。
- 图片默认由浏览器直接访问 Wikimedia Commons 的图片地址，**不经过 Go 或本项目 Nginx 转发**。
- 数学公式使用本地打包的 KaTeX 排版，不是逐个下载公式图片。
- 图片懒加载；前端构建包含长缓存哈希资源；Nginx 对正文 JSON、JS、CSS 启用 gzip。
- 图片暂未上传任何云存储，也未创建收费资源。
- 当前已渲染图片清单去重为 8,370 个文件名，保存在 `backend/.cache/wiki-media-inventory.json`；只是清单，不代表全部可下载或具有已核验的再分发授权，也不包括未展开模板里的图片。

## 三种路径的区别

| 方案 | 图片路径 | 对业务服务器带宽的影响 | 主要限制 |
|---|---|---|---|
| 当前直连 | 浏览器 → Wikimedia | 不占业务服务器图片传输带宽 | 受用户访问维基网络条件影响；不可保证稳定 |
| 本机存图或代理 | 浏览器 → 业务服务器 → 图片 | 占业务服务器出站带宽；无缓存代理还会产生入站流量 | 容易占满小带宽机器 |
| 对象存储 + CDN | 浏览器 → CDN；CDN 未命中 → 对象存储 | 正常读图不经过业务服务器 | 存储、请求、CDN 下行和可能的回源流量仍收费 |

推荐正式部署使用第三种。CDN 源站应设置为对象存储，不要指向 Go 服务器；也不要用 `/api/image-proxy` 把图片再中转回来。

对象存储解决图片托管，CDN 提供边缘分发；并不是“使用对象存储之后没有流量费用”。按腾讯云文档，直连 COS 下载与通过 CDN 下载分别对应不同计费路径，CDN 缓存命中主要减少回源流量，不能消除终端下载流量。

参考：[COS 流量费用](https://cloud.tencent.com/document/product/436/53863)、[COS-CDN 计费说明](https://cloud.tencent.com/document/product/228/37849)。价格依地域、套餐、计费模式而异，此处不预估单价。

## 容量与流量估算

假设每天 1,000 位访客，每人阅读 10 页，每页实际加载 3 张、每张平均 150 KB：

`1,000 × 10 × 3 × 150 KB ≈ 4.5 GB/天 ≈ 135 GB/月`

这是忽略浏览器缓存、重复图片、机器人和其他静态文件的示例，不是当前网站的实测数据。峰值还需看并发，例如每秒加载 10 页、每页 450 KB 图片，瞬时传输需求约为 36 Mbps。月流量配额和峰值带宽是两个不同的限制。

建议采用适当尺寸的缩略图、WebP/AVIF（线图保留 SVG 时注意 SVG 安全）、浏览器长缓存、图片懒加载、CDN 缓存；设置预算告警及流量告警。Referer 防盗链只能减少普通盗链，不能代替访问控制或费用上限。

## 已准备的前端切换入口

本地 Vite 在 `frontend/.env.local` 配置；Docker 构建在项目根 `.env` 配置同名变量并重新构建 web。所有 `VITE_*` 内容都会公开在前端中，**绝不能放 SecretId、SecretKey、数据库密码或长期签名凭证**。

```dotenv
VITE_WIKI_IMAGE_MODE=mirror-only
VITE_MEDIA_MANIFEST_URL=/media-manifest.json
VITE_MEDIA_ALLOWED_ORIGINS=https://img.example.com
```

模式：

- `external`：默认；有合法镜像记录时优先 CDN，否则浏览器直连 Commons。
- `mirror-only`：仅加载已配置镜像；缺失时保留图注/来源链接，不偷偷回源或占用业务服务器流量。
- `off`：不下载图片，保留图注/来源链接，适合低流量演示。

完成图片上传后，将映射放在 `frontend/public/media-manifest.json`（需自行创建），键为原始文件名，空格替换为下划线。示例中的域名和作者仅是占位符，必须换成实际数据：

```json
{
  "Example_image.png": {
    "url": "https://img.example.com/wiki/sha256/720.webp",
    "author": "原文件署名作者",
    "license": "该图片实际适用的许可证",
    "licenseUrl": "https://example.com/actual-license",
    "sourceUrl": "https://commons.wikimedia.org/wiki/File:Example_image.png"
  }
}
```

也可把映射文件托管在 CDN，`VITE_MEDIA_MANIFEST_URL` 使用完整 HTTPS 地址，必须把其源站加入白名单，且需要允许网站 Origin 的 CORS 读取。普通 `<img>` 显示本身不需要 CORS；画布读取或跨域 fetch 则需要。

镜像域名和路径必须是已经部署的真实 HTTPS 地址；镜像模式不负责自动上传或猜测文件是否存在。文件名映射能保留既有 WikiText，不必再次修改数据库中的每个图片地址。

## 上传和公开部署前仍需完成

1. 选择云厂商、地域、月度预算和图片域名，再创建对象存储与 CDN；当前没有开通，也没有收集云密钥。
2. 按源图获取作者、许可、源文件页等元数据。不能把词条的 CC BY-SA 4.0 标签直接套在每张图片上；有些中文维基本地文件不是 Commons 文件，需单独核查。Commons 图源热链同样须遵守许可要求。[Commons 复用说明](https://commons.wikimedia.org/wiki/Commons:Reusing_content_outside_Wikimedia)
3. 对可再分发图片生成适当大小的派生文件，上传后设置正确 Content-Type、缓存头和访问策略，并生成带作者/许可的映射。公开内容可采用私有桶 + CDN 回源授权；不要公开写权限。
4. 测试真实服务器区域和目标用户网络环境，检查图片状态码、缓存命中、费用统计。
5. **当前项目仍是本地开发 MVP，不能直接使用开发配置公开部署。** 已支持账户认证、用户隔离、批注审核和登录注册限流，详见 [账户说明](accounts.md)。上线仍需补充可信网关限流、反垃圾与管理员审计策略。不要把开发 compose 中的 MySQL 3306、ES 9200、Go 8080 直接暴露公网，不要使用示例密码。配置 HTTPS 和 `AUTH_COOKIE_SECURE=true`，并把数据库放内网。

## 内容修复范围

5,000 篇原始 WikiText 已离线重新解析：完整导言、段落、列表、标题层级、参考文献、图片结构、表格和公式。复杂模板/Lua 模块未全部展开，原文保留在“待解析模板”中；KaTeX 不支持的公式保留原始 TeX 而不伪造结果。源码解析失败也明确提示并保留源片段。

浏览器实际页面抽查不能代替全库人工校审。本轮未用 AI 编造或补写原文。

## 复现与备份

- `scripts/render_wiki.py --input <原始JSONL> --output <新文件>`：不覆盖原始导出。
- 在加载项目 `.env` 的进程环境中运行 `go run ./cmd/repair-content --input <解析结果>`，原位更新已有章节，保留旧章节 ID；废弃结构隐藏而不删除。
- 首次修复前完整备份：`backend/.cache/mathwiki-before-render-v2.sql`。
- 最终版写入前再次备份：`backend/.cache/mathwiki-before-render-release-20260917.sql`。
- 数据库备份包含本地用户内容，不应上传公共仓库；恢复需停写并核对目标，不能直接覆盖正在使用的库。
