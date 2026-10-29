import { reactive } from 'vue'
import { conversationHistory, type ConversationTurn, type StreamEvent } from './explanationStream.ts'

export type ChatWindow = {
  id: number; name: string; prompt: string; audience: string; style: string
  minimized: boolean; x: number; y: number
  answerMode: 'balanced' | 'strict'; webSearch: boolean
}
export type ChatTurn = ConversationTurn & {
  id: number; windowId: number; windowName: string; articleId: number
  audience: string; style: string
  answerMode: 'balanced' | 'strict'; webSearch: boolean
  status: 'queued' | 'running' | 'done' | 'error' | 'cancelled'
  phase: string
}
type ChatInput = {
  articleId: number; question: string; audience: string; style: string
  answerMode: 'balanced' | 'strict'; webSearch: boolean
  history: { role: string; content: string }[]
}
type Options = {
  articleId: () => number
  request: (input: ChatInput, event: (event: StreamEvent) => void, signal: AbortSignal) => Promise<void>
  onUpdate?: (windowId: number, force?: boolean) => void
}

export function createSharedChats(options: Options) {
  const state = reactive({
    windows: [{ id: 1, name: '主窗口', prompt: '', audience: '本科生', style: '直觉 + 严谨', minimized: false, x: 0, y: 0, answerMode: 'balanced', webSearch: false }] as ChatWindow[],
    turns: [] as ChatTurn[], activeWindowId: 1
  })
  let windowSequence = 1, turnSequence = 0, epoch = 0
  let active: { controller: AbortController; turnId: number } | null = null
  const pending = (windowId: number) => state.turns.find(turn => turn.windowId === windowId && (turn.status === 'queued' || turn.status === 'running'))
  const turnsFor = (windowId: number) => state.turns.filter(turn => turn.windowId === windowId).slice(-20)
  const sharedHistory = () => conversationHistory(state.turns.filter(turn => turn.status === 'done'))

  async function pump() {
    if (active) return
    const turn = state.turns.find(item => item.status === 'queued')
    if (!turn) return
    const owner = state.windows.find(window => window.id === turn.windowId)
    if (!owner) { turn.status = 'cancelled'; void pump(); return }
    const generation = epoch
    const controller = new AbortController()
    active = { controller, turnId: turn.id }
    turn.status = 'running'
    const valid = () => generation === epoch && !controller.signal.aborted && turn.status === 'running'
    try {
      // Build history at dequeue time: an earlier window's completed answer must
      // be available to a question queued while that answer was still streaming.
      await options.request({ articleId: turn.articleId, question: turn.question, audience: turn.audience, style: turn.style, answerMode: turn.answerMode, webSearch: turn.webSearch, history: sharedHistory() }, event => {
        if (!valid()) return
        if (event.type === 'delta') turn.text += event.text ?? ''
        if (event.type === 'phase') turn.phase = event.text ?? 'generating'
        if (event.type === 'done') turn.result = event.explanation
        options.onUpdate?.(turn.windowId)
      }, controller.signal)
      if (valid()) {
        if (!turn.result) throw new Error('回答未完成，请重试')
        turn.status = 'done'
      }
    } catch (error) {
      if (generation === epoch && turn.status === 'running') {
        turn.result = undefined
        turn.status = controller.signal.aborted ? 'cancelled' : 'error'
        turn.error = controller.signal.aborted ? '已停止。以上内容尚未完成核验。' : error instanceof Error ? error.message : '解释生成失败，请重试'
        if (!owner.prompt) owner.prompt = turn.question
      }
    } finally {
      if (active?.controller === controller) active = null
      options.onUpdate?.(turn.windowId)
      void pump()
    }
  }

  function submit(windowId: number) {
    const owner = state.windows.find(window => window.id === windowId)
    if (!owner || pending(windowId)) return false
    const question = owner.prompt.trim()
    if (!question || [...question].length > 2000) return false
    // Bounded page memory; only the latest six completed model turns are sent.
    if (state.turns.length >= 80) {
      const oldest = state.turns.findIndex(turn => turn.status !== 'queued' && turn.status !== 'running')
      if (oldest >= 0) state.turns.splice(oldest, 1)
    }
    state.turns.push({ id: ++turnSequence, windowId, windowName: owner.name, articleId: options.articleId(), question, audience: owner.audience, style: owner.style, answerMode: owner.answerMode, webSearch: owner.webSearch, text: '', status: 'queued', phase: 'generating' })
    owner.prompt = ''
    options.onUpdate?.(windowId, true)
    void pump()
    return true
  }

  function cancel(windowId: number) {
    const turn = pending(windowId)
    if (!turn) return
    const owner = state.windows.find(window => window.id === windowId)
    if (owner && !owner.prompt) owner.prompt = turn.question
    if (turn.status === 'queued') {
      turn.status = 'cancelled'
      turn.error = '已取消排队，尚未调用模型。'
    } else if (active?.turnId === turn.id) {
      active.controller.abort()
      turn.status = 'cancelled'
      turn.result = undefined
      turn.error = '已停止。以上内容尚未完成核验。'
    }
    options.onUpdate?.(windowId)
  }

  function addWindow(x = 24, y = 100) {
    if (state.windows.length >= 4) return null
    const main = state.windows[0]!
    const id = ++windowSequence
    state.windows.push({ id, name: `窗口 ${id}`, prompt: '', audience: main.audience, style: main.style, minimized: false, x, y, answerMode: main.answerMode, webSearch: false })
    state.activeWindowId = id
    return id
  }

  function closeWindow(windowId: number) {
    if (windowId === 1) return
    cancel(windowId)
    state.windows = state.windows.filter(window => window.id !== windowId)
    if (state.activeWindowId === windowId) state.activeWindowId = 1
    // Completed turns stay in the shared context after their window is closed.
  }

  function clear() {
    ++epoch
    active?.controller.abort()
    state.turns = []
    for (const window of state.windows) { window.prompt = ''; window.webSearch = false }
  }

  return { state, pending, turnsFor, sharedHistory, submit, cancel, addWindow, closeWindow, clear }
}
