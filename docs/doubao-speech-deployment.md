# 豆包语音输入部署

接入豆包流式语音识别模型 2.0 小时版，资源 ID 固定为 `volc.seedasr.sauc.duration`。使用一句话识别 WebSocket 接口，录音时分段发送，结束后得到文字填入输入框，用户确认后才发送给 DeepSeek。无本地语音模型、无音频落盘、无公开音频链接。

## 计费（2026-10-11 核对）

官方[计费说明](https://docs.volcengine.com/docs/DoubaoVoice/Billinginstructions-21?lang=zh)：流式识别 2.0 按量 1 元/小时，录音文件识别 2.0 按量 0.8 元/小时。时长按每次调用的音频累计到秒再换算，不把每条短录音都按一小时收费。按量模式没有调用则无识别用量费用。资源包需要预先付费，并发增购包按月收费，与识别用量费分开。

录音文件 0.8 元规格是异步任务，文档提示通常 3 小时内完成；此项目按用户选择使用 1 元的流式规格。官方[计费概述](https://docs.volcengine.com/docs/DoubaoVoice/BillingOverview-15?lang=zh)列出语音识别试用 20 小时、半年有效，实际以控制台领取页面为准。试用用尽或正式开通后的计费状态需在控制台确认。

## 控制台和后端配置

在[豆包语音新版控制台](https://console.volcengine.com/speech/new/overview?projectName=default)完成认证，领取或开通“豆包流式语音识别模型2.0”，选择按量小时版；在 API Key 管理生成语音 API Key。它不是火山方舟或 DeepSeek 的密钥。

在服务器 `/opt/mathwiki/.env` 追加或修改（保留数据库和现有 AI 配置）：

```dotenv
STT_PROVIDER=doubao
DOUBAO_SPEECH_API_KEY=在服务器直接填入你的语音APIKey
```

不要把真实密钥提交 GitHub 或填到浏览器前端。后端启动时读取配置，修改后需重启 Supervisor。未配置密钥时，网页会提示语音未开通。

## HTTPS 与 Nginx

网页麦克风要求安全上下文：生产必须使用 HTTPS（localhost 本地开发例外）。原 HTTP IP 页面不能录音。启用有效 HTTPS 后设置 `AUTH_COOKIE_SECURE=true`。

在当前站点的 `server` 中增加以下块，保留原 `/api/` 代理：

```nginx
location = /api/v1/speech/stream {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $http_host;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 90s;
    proxy_send_timeout 90s;
    proxy_buffering off;
}
```

`$http_host` 保留端口，供严格同源校验使用。运行 `/www/server/nginx/sbin/nginx -t`，通过后运行 `/www/server/nginx/sbin/nginx -s reload`。本功能不需要新增公网端口。

## 更新包与服务器编译

将 `mathwiki-doubao-speech-update.zip` 上传至 `/opt` 并解压覆盖，保留包内 `mathwiki/` 目录。包包含前面的 Enter 发送、流式 AI 对话和学习风格更新，保留服务器 `.env`。前后端均需重新编译：

```bash
(
set -e
source /etc/profile.d/mathwiki-go.sh
source /etc/profile.d/mathwiki-node.sh
cd /opt/mathwiki/backend
GOMAXPROCS=1 GOMEMLIMIT=256MiB CGO_ENABLED=0 go build -p 1 -trimpath -ldflags="-s -w" -o bin/mathwiki-server.new ./cmd/server
chmod 755 bin/mathwiki-server.new
cd /opt/mathwiki/frontend
pnpm install --frozen-lockfile
NODE_OPTIONS=--max-old-space-size=384 pnpm build
mv -f /opt/mathwiki/backend/bin/mathwiki-server.new /opt/mathwiki/backend/bin/mathwiki-server
)
```

成功后在 Supervisor 重启 `mathwiki`，通过 HTTPS 登录并硬刷新。点击麦克风允许权限，说一句话，再次点击停止；识别结果进入输入框，确认文字再发送。

## 验证与限制

本地测试验证 PCM 降采样、时长限制、二进制协议、gzip 回复和 WebSocket 模拟服务；没有真实密钥，尚未验证真实账户调用和扣费。生产需做一次短录音测试并在火山控制台核对用量。

登录用户每分钟最多开启 10 次，服务器最多同时 2 路，每路最多 60 秒音频；异常、切换词条、退出登录和离开页面都会关闭录音和连接。录音包含的静音仍属于送给服务的音频时长。费用限制需另在控制台配置额度预警，代码限流不是账户支出上限。

接口依据：[一句话识别](https://docs.volcengine.com/docs/DoubaoVoice/unidirectional-streaming-automatic-speech-recognition-websocket?lang=zh)、[二进制协议](https://docs.volcengine.com/docs/DoubaoVoice/LargemodelstreamingautomaticspeechrecognitionAPI?lang=zh)。
