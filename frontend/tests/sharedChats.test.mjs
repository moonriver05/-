import test from 'node:test'
import assert from 'node:assert/strict'
import { createSharedChats } from '../src/sharedChats.ts'

const tick = () => new Promise(resolve => setImmediate(resolve))
const result = summary => ({ mode: 'model', summary, steps: [], citations: [], confidence: .8, checks: [] })
function session({ ignoreAbort = false } = {}) {
  const calls = []
  let articleId = 7
  const chats = createSharedChats({ articleId: () => articleId, request(input, event, signal) {
    return new Promise((resolve, reject) => {
      const call = { input, event, signal, resolve, reject }
      calls.push(call)
      if (!ignoreAbort) signal.addEventListener('abort', () => reject(new DOMException('Stopped', 'AbortError')), { once: true })
    })
  } })
  const ask = (id, question) => { chats.state.windows.find(window => window.id === id).prompt = question; return chats.submit(id) }
  const finish = (index, summary) => { calls[index].event({ type: 'done', explanation: result(summary) }); calls[index].resolve() }
  return { chats, calls, ask, finish, setArticle: value => { articleId = value } }
}

test('different windows have separate views and use completed shared context when dequeued', async () => {
  const { chats, calls, ask, finish } = session()
  const second = chats.addWindow()
  ask(1, '解释加法')
  calls[0].event({ type: 'delta', text: '初稿' })
  ask(second, '给它举个生活例子')
  assert.equal(calls.length, 1, 'one model request at a time')
  assert.equal(chats.pending(second).status, 'queued')
  assert.equal(chats.turnsFor(1)[0].question, '解释加法')
  assert.equal(chats.turnsFor(second)[0].question, '给它举个生活例子')
  finish(0, '加法是合并数量。'); await tick()
  assert.equal(calls.length, 2)
  assert.deepEqual(calls[1].input.history.map(item => item.content), ['解释加法', '加法是合并数量。'])
  finish(1, '两只苹果和三只苹果合成五只。'); await tick()
  ask(1, '刚才苹果的例子可以用符号表示吗？')
  assert.equal(calls[2].input.history.length, 4)
  assert.match(calls[2].input.history[3].content, /苹果/)
  finish(2, '2+3=5'); await tick()
})

test('cancel queued questions without calling the API; closing another window does not cancel the running one', async () => {
  const { chats, calls, ask, finish } = session()
  const second = chats.addWindow(), third = chats.addWindow()
  ask(1, '运行中的问题'); ask(second, '取消这个排队问题'); ask(third, '下一个问题')
  chats.closeWindow(second)
  assert.equal(calls[0].signal.aborted, false)
  assert.equal(chats.state.turns.find(t => t.windowId === second).status, 'cancelled')
  finish(0, '已完成'); await tick()
  assert.equal(calls.length, 2)
  assert.equal(calls[1].input.question, '下一个问题')
  assert.equal(calls[1].input.history.some(m => m.content.includes('排队问题')), false)
  finish(1, '下一轮完成'); await tick()
})

test('closing an active window aborts only its request; completed answers survive window closure', async () => {
  const { chats, calls, ask, finish } = session()
  const second = chats.addWindow()
  ask(second, '完成的知识'); finish(0, '保留在共享上下文里'); await tick()
  chats.closeWindow(second)
  ask(1, '接着讲')
  assert.equal(calls[1].input.history[1].content, '保留在共享上下文里')
  chats.cancel(1); await tick()
  assert.equal(calls[1].signal.aborted, true)
  assert.equal(chats.sharedHistory().length, 2)
  const third = chats.addWindow()
  ask(third, '关闭这个正在生成的窗口'); ask(1, '继续处理主窗口的问题')
  chats.closeWindow(third); await tick()
  assert.equal(calls[2].signal.aborted, true)
  assert.equal(calls[3].input.question, '继续处理主窗口的问题')
  assert.equal(calls[3].input.history[1].content, '保留在共享上下文里')
  finish(3, '正常继续'); await tick()
})

test('reset cancels all queued work and ignores late results before starting the new article', async () => {
  const { chats, calls, ask, finish, setArticle } = session({ ignoreAbort: true })
  const second = chats.addWindow()
  ask(1, '旧词条问题'); ask(second, '旧队列问题')
  chats.clear(); setArticle(9)
  ask(second, '新词条问题')
  assert.equal(calls[0].signal.aborted, true)
  assert.equal(calls.length, 1, 'wait for the old request to release before starting another')
  finish(0, '旧结果必须忽略'); await tick()
  assert.equal(calls[1].input.articleId, 9)
  assert.deepEqual(calls[1].input.history, [])
  finish(1, '新词条回答'); await tick()
  assert.equal(chats.state.turns.length, 1)
  assert.equal(chats.state.turns[0].result.summary, '新词条回答')
})

test('failed and demo responses are excluded; history and memory remain bounded', async () => {
  const chats = createSharedChats({ articleId: () => 1, request: async (input, emit) => {
    if (input.question === '失败') throw new Error('模拟失败')
    emit({ type: 'done', explanation: { ...result(input.question), mode: input.question === '演示' ? 'demo' : 'model' } })
  } })
  for (const question of ['失败', '演示', ...Array.from({ length: 90 }, (_, index) => `问题${index}`)]) {
    chats.state.windows[0].prompt = question
    assert.equal(chats.submit(1), true)
    await tick()
  }
  assert.equal(chats.state.turns.length, 80)
  assert.equal(chats.turnsFor(1).length, 20)
  assert.equal(chats.sharedHistory().length, 12)
  assert.equal(chats.sharedHistory()[0].content, '问题84')
  assert.ok(chats.addWindow()); assert.ok(chats.addWindow()); assert.ok(chats.addWindow())
  assert.equal(chats.addWindow(), null)
})
