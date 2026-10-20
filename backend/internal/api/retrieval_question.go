package api

import (
	"regexp"
	"strings"

	"mathwiki/internal/ai"
)

var followupReference = regexp.MustCompile(`第[一二三四五六七八九十百0-9]+[步点条项个]|这一步|那一步|上一步|下一步|这个|那个|这里|那里|这样|那样|这些|那些|它|上面|前面|刚才|继续|再讲|再说|再解释|再简单|简单.*说|说简单|没[听看]?懂|不明白|举[一个些]*例|换[一个种]*[例说]|我是小学生|用.*(短句|简单|类比)|能.*(例子|简单)|^为什么[？?]?$|为什么呢|然后呢`)

// Add bounded conversational context only for references/re-explanations. This is
// embedding input, never new factual evidence or instructions for the answer model.
// Explicit standalone questions keep their own retrieval topic.
func retrievalQuestion(question string, history []ai.Message) string {
	question = strings.TrimSpace(question)
	if len(history) < 2 || !followupReference.MatchString(question) {
		return question
	}
	start := len(history) - 4
	if start < 0 {
		start = 0
	}
	var context strings.Builder
	for i := start; i < len(history); i++ {
		message := history[i]
		if message.Role == "user" {
			context.WriteString("\n此前的问题：" + retrievalExcerpt(message.Content, 400))
		} else if message.Role == "assistant" && i == len(history)-1 {
			context.WriteString("\n上一轮回答（仅供定位追问，不是事实证据）：" + retrievalExcerpt(message.Content, 1600))
		}
	}
	return "本轮问题：" + question + context.String() + "\n检索目标：查找能回答本轮追问的知识库原文。"
}

func retrievalExcerpt(text string, limit int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) > limit {
		return string(runes[:limit])
	}
	return string(runes)
}
