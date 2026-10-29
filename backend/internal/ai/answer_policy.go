package ai

const BalancedAnswer = "balanced"
const StrictAnswer = "strict"

func answerMode(options []string) string {
	if len(options) > 0 && options[0] == BalancedAnswer {
		return BalancedAnswer
	}
	return StrictAnswer // Retain the existing library contract for older callers.
}

func answerGuidance(mode string) string {
	math := `数学排版：行内公式用 $...$，独立公式用 $$...$$；上下标使用花括号，使用合法 LaTeX。输出 JSON 时正确转义反斜杠，不能把长公式写成无法识别的裸 TeX。`
	if mode == BalancedAnswer {
		return `回答范围为“智能讲解”：优先用提供的资料回答；资料没有逐字写出推导时，可以使用通行的数学定义、链式法则、代数变形和必要背景知识补全解释。不要仅因原文未写完整证明就中断讲解。
明确给出推导的假设、符号与步骤；把原文直接支持的部分与“推导补充”或“背景补充”区别说明。不能把模型记忆冒充联网搜索结果，不能用无关引文给补充结论背书。无法确认的新近事实、实验结果或来源时明确说明不确定。
citations 只填写能逐字在所给资料中找到的短句；没有可用引文时返回空数组，并在答案中说明“以下为模型讲解，暂无可核查引文”。网页的搜索摘要不等于已核对完整正文。核验阶段检查数学推理、成立条件和引用，保留合理的推导补充；不要机械删除所有未在原文逐字出现的推导。` + "\n" + math
	}
	return `回答范围为“严格依据资料”：事实与结论必须有提供的资料支持；证据不足时说明缺少什么。只引用资料原句，不能编造引用或把模型背景知识冒充原文。` + "\n" + math
}
