import type { Explanation } from './types'

export type StreamEvent = { type: 'phase' | 'delta' | 'done' | 'error'; text?: string; explanation?: Explanation }
export type ConversationTurn = { question: string; text: string; result?: Explanation; error?: string }

export function conversationHistory(turns: ConversationTurn[]) {
  const pairs = turns.filter(turn => turn.result?.mode === 'model').slice(-6).map(turn => {
    const result = turn.result!
    const answer = [result.summary, ...result.steps.map(step => `${step.title}：${step.content}`)].join('\n')
    return [{ role: 'user', content: turn.question }, { role: 'assistant', content: Array.from(answer).slice(0, 8000).join('') }]
  })
  while (pairs.reduce((sum, pair) => sum + pair.reduce((n, message) => n + Array.from(message.content).length, 0), 0) > 32000) pairs.shift()
  return pairs.flat()
}

export async function readExplanationStream(response: Response, onEvent: (event: StreamEvent) => void): Promise<void> {
  if (!response.body) throw new Error('浏览器无法读取流式响应')
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let pending = ''
  let data: string[] = []
  let completed = false
  const dispatch = () => {
    if (!data.length) return
    const event = JSON.parse(data.join('\n')) as StreamEvent
    data = []
    if (event.type === 'error') throw new Error(event.text || '解释生成失败')
    if (event.type === 'done') {
      if (!event.explanation || !Array.isArray(event.explanation.steps) || !Array.isArray(event.explanation.citations)) throw new Error('回答数据不完整')
      completed = true
    }
    onEvent(event)
  }
  try {
    while (!completed) {
      const { value, done } = await reader.read()
      pending += decoder.decode(value, { stream: !done })
      let newline: number
      while ((newline = pending.indexOf('\n')) !== -1) {
        const line = pending.slice(0, newline).replace(/\r$/, '')
        pending = pending.slice(newline + 1)
        if (!line) dispatch()
        else if (line.startsWith('data:')) data.push(line.slice(5).replace(/^ /, ''))
        if (completed) break
      }
      if (done) break
    }
    if (!completed) throw new Error('连接已断开，回答未完成，请重试')
  } finally {
    await reader.cancel().catch(() => {})
    reader.releaseLock()
  }
}
