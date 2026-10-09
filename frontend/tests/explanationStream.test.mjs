import test from 'node:test'
import assert from 'node:assert/strict'
import { readExplanationStream, conversationHistory } from '../src/explanationStream.ts'

const answer = { mode: 'model', summary: '核验结果', steps: [], citations: [], checks: [], confidence: .8 }
const event = (data) => `data: ${JSON.stringify(data)}\r\n\r\n`

test('decode fragmented SSE including split Chinese UTF-8 and deliver deltas before done', async () => {
  const bytes = new TextEncoder().encode(event({ type: 'delta', text: '加法😀' }) + event({ type: 'done', explanation: answer }))
  const stream = new ReadableStream({ start(controller) { for (const byte of bytes) controller.enqueue(Uint8Array.of(byte)); controller.close() } })
  const events = []
  await readExplanationStream(new Response(stream), (e) => events.push(e))
  assert.equal(events[0].text, '加法😀')
  assert.equal(events[1].explanation.summary, '核验结果')
})

test('connection loss and explicit error do not report success', async () => {
  await assert.rejects(readExplanationStream(new Response(event({ type: 'delta', text: '未完成' })), () => {}), /未完成/)
  await assert.rejects(readExplanationStream(new Response(event({ type: 'error', text: '模型不可用' })), () => {}), /模型不可用/)
})

test('only completed model answers enter bounded follow-up history', () => {
  const turns = Array.from({ length: 10 }, (_, i) => ({ question: `问题${i}`, text: '', result: { ...answer, summary: `回答${i}` } }))
  turns.push({ question: '停止的回答', text: '半句', error: '已停止' }, { question: '演示', text: '', result: { ...answer, mode: 'demo' } })
  const history = conversationHistory(turns)
  assert.equal(history.length, 12)
  assert.equal(history[0].content, '问题4')
  assert.equal(history.at(-1).content, '回答9')
  assert.deepEqual(history.map(x => x.role), Array.from({ length: 6 }, () => ['user', 'assistant']).flat())
  const long = conversationHistory(Array.from({ length: 6 }, () => ({ question: '追问', result: { ...answer, summary: '文'.repeat(9000) } })))
  assert.ok(long.reduce((n, x) => n + Array.from(x.content).length, 0) <= 32000)
})
