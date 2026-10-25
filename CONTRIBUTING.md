# 贡献与代码规范

- 从 main 创建功能分支；说明问题、最终行为、验证与限制，不混入无关重构。
- 后端运行 gofmt、go test ./...、go vet ./...；前端运行 pnpm test、pnpm build。测试覆盖权限、取消、降级与数据边界。
- cmd 编排入口，internal 保持协议边界；TypeScript 保持类型完整，不绕过 HTML 清洗。
- 注释解释为何回查公开内容、为何不信任模型行号、为何降级，避免逐行翻译代码。
- 禁止提交真实 .env、令牌、数据库、账户、邮件、日志与上传文件。PowerShell 用 scripts/check-secrets.ps1；跨平台 CI 用 scripts/check-public-files.py。
- 内容保留来源与许可，社区投稿审核通过后才公开或纳入检索。模拟测试不代表真实模型质量或生产并发已达标。
