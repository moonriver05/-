package api

import (
	"strings"
	"testing"
	"unicode/utf8"

	"mathwiki/internal/ai"
)

func TestRetrievalQuestionResolvesReferencesWithoutPollutingNewTopics(t *testing.T) {
	history := []ai.Message{
		{Role: "user", Content: "归并排序如何工作？"},
		{Role: "assistant", Content: "第一步拆分。第二步合并有序数组，比较两端最小元素。"},
	}
	for _, question := range []string{"第二步为什么这样？", "能举个例子吗？", "没听懂，再简单些", "我是小学生，用短句解释"} {
		got := retrievalQuestion(question, history)
		if !strings.Contains(got, question) || !strings.Contains(got, "归并排序") || !strings.Contains(got, "合并有序数组") {
			t.Errorf("follow-up lost its subject: %s", got)
		}
	}
	question := "勾股定理的成立条件是什么？"
	if got := retrievalQuestion(question, history); got != question {
		t.Fatalf("independent topic polluted by sorting history: %s", got)
	}
	if got := retrievalQuestion("第二步为什么？", nil); got != "第二步为什么？" {
		t.Fatal("invented context without history")
	}
}

func TestRetrievalQuestionBoundsHistoryAndPreservesCurrentQuestion(t *testing.T) {
	history := []ai.Message{
		{Role: "user", Content: "旧主题不应进入检索"}, {Role: "assistant", Content: "旧回答不应进入检索"},
		{Role: "user", Content: "前向错误更正" + strings.Repeat("甲", 8000)}, {Role: "assistant", Content: "中间回答不应进入检索"},
		{Role: "user", Content: "冗余如何恢复数据？" + strings.Repeat("乙", 8000)},
		{Role: "assistant", Content: "第一步编码。第二步恢复。" + strings.Repeat("丙", 8000)},
	}
	question := "第二步能再解释吗？"
	got := retrievalQuestion(question, history)
	for _, unwanted := range []string{"旧主题", "旧回答", "中间回答"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("unneeded history included: %s", unwanted)
		}
	}
	if !strings.HasPrefix(got, "本轮问题："+question) || !strings.Contains(got, "前向错误更正") || !strings.Contains(got, "第二步恢复") || !utf8.ValidString(got) || utf8.RuneCountInString(got) > 2600 {
		t.Fatalf("invalid or unbounded contextual query: %d runes", utf8.RuneCountInString(got))
	}
}
