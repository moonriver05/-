# 学习程度与解释风格更新

生成与核验现在共用明确的表达规则，核验只纠正事实、逻辑和引用，不统一改成同一套正式讲解。当前选择优先于旧回答的难度与风格。

小学生使用日常词、短句及具体事物，专业学习者使用精确定义、假设和关键推理。生活类比要求具体情景、对应关系及类比局限；直觉加严谨要求先解释直觉，再给出定义或推理。高中生、本科生、公式推导、例题演示也各有对应要求。

称呼确认、简单事实或寒暄保持简短，不强行编造类比或公式。不同模式改变表达方式，不改变事实与证据要求。实际表现仍需用真实模型比较，模拟测试只验证各阶段正确传递表达要求。

## 部署

上传 `mathwiki-teaching-style-update.zip` 至 `/opt`，解压覆盖源码，保留 `mathwiki/` 目录。只需重编译后端：

```bash
(
set -e
source /etc/profile.d/mathwiki-go.sh
cd /opt/mathwiki/backend
GOMAXPROCS=1 GOMEMLIMIT=256MiB CGO_ENABLED=0 go build -p 1 -trimpath -ldflags="-s -w" -o bin/mathwiki-server.new ./cmd/server
chmod 755 bin/mathwiki-server.new
mv -f bin/mathwiki-server.new bin/mathwiki-server
)
```

成功后在 Supervisor 重启 `mathwiki`。

## 对照体验

选择 AlphaGo 词条，每次先点击“开始新对话”，用同一问题“AlphaGo 是怎样决定下一步棋的？请解释过程”比较“小学生 + 生活类比”和“专业学习者 + 直觉 + 严谨”。观察术语、前置知识、例子和推理结构是否有区别。再在已有对话中切换程度与风格追问，确认当前选择生效。不要用“这个狗是什么狗”这种简单辨认问题判断推导风格差异。
