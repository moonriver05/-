import type { Explanation } from './types'

export function answerBadge(answer: Explanation) {
  if (answer.mode === 'demo') return '引用示例 · 未调用模型'
  if (answer.grounding === 'model') return '模型讲解 · 暂无可核查引文'
  if (answer.answerMode === 'balanced') return '模型复核完成 · 含推导补充'
  return '模型自检完成'
}
export function groundingNotice(answer: Explanation) {
  if (answer.grounding === 'model') return '以下为模型推导或背景讲解，尚无有效引文支持，请核对前提和关键结论。'
  if (answer.answerMode === 'balanced') return '引用已与提供的资料匹配；模型补充与推导仍需核对，不代表原文逐项证明。'
  return ''
}
export function webSearchNotice(answer: Explanation) {
  if (!answer.webSearch) return ''
  const fallback = answer.answerMode === 'strict' ? '本次仅使用已有资料。' : '本次使用知识库与模型讲解。'
  if (answer.webSearch.status === 'failed') return '联网搜索暂不可用，' + fallback
  if (answer.webSearch.status === 'empty') return '联网搜索未取得可用资料，' + fallback
  return `联网补充 · ${answer.webSearch.count} 份网页资料；网页来源不代表结论已被证明。`
}
