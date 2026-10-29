import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { JSDOM } from 'jsdom'
import { parse, compileScript } from 'vue/compiler-sfc'
import ts from 'typescript'

const dom = new JSDOM('<!doctype html><meta name="theme-color"><div id="app"></div>', { url: 'http://localhost/', pretendToBeVisual: true })
for (const name of ['window', 'document', 'Document', 'Element', 'HTMLElement', 'SVGElement', 'Node', 'Event', 'DOMException', 'localStorage']) globalThis[name] = dom.window[name]
globalThis.requestAnimationFrame = window.requestAnimationFrame.bind(window)
globalThis.cancelAnimationFrame = window.cancelAnimationFrame.bind(window)
globalThis.getComputedStyle = window.getComputedStyle.bind(window)
window.matchMedia = () => ({ matches: true })
window.confirm = () => true
HTMLElement.prototype.scrollTo = () => {}
HTMLElement.prototype.scrollIntoView = () => {}
const { createApp, nextTick } = await import('vue')
const { api } = await import('../src/api.ts')
const { articles } = await import('../src/mock.ts')
function compile(file, transform = source => source) {
  const url = new URL(file, import.meta.url)
  const { descriptor } = parse(readFileSync(url, 'utf8'))
  const source = transform(compileScript(descriptor, { id: file, inlineTemplate: true }).content)
    .replace(/from ['"]([^'"]+)['"]/g, (_, path) => `from ${JSON.stringify(path.startsWith('.') ? new URL(path + '.ts', url).href : import.meta.resolve(path))}`)
  const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ESNext } }).outputText
  return `data:text/javascript;base64,${Buffer.from(js).toString('base64')}`
}
const popupURL = compile('../src/components/AiChatWindow.vue')
const App = (await import(compile('../src/App.vue', source => source
  .replace(/import AiChatWindow from ['"]\.\/components\/AiChatWindow.vue['"]/, `import AiChatWindow from '${popupURL}'`)
  .replace(/import (\w+) from ['"]\.\/components\/[^'"]+\.vue['"]/g, 'const $1 = { render: () => null }')))).default
const flush = async () => { await new Promise(resolve => setTimeout(resolve, 15)); await nextTick() }
function fill(input, text) { input.value = text; input.dispatchEvent(new Event('input', { bubbles: true })) }
const answer = summary => ({ mode: 'model', summary, steps: [], citations: [], checks: [], confidence: .8 })

test('real popup UI queues with shared context, keeps separate turns, restores drafts and clears the whole session', async () => {
  const saved = { ...api }, calls = []
  api.me = async () => ({ user: { id: 1, name: '学习者', role: 'learner' } })
  api.getStats = async () => ({ articleCount: 2, sectionCount: 4 })
  api.getCategories = async () => []
  api.getCapabilities = async () => ({ aiConfigured: true, speechConfigured: false })
  api.listArticles = async () => ({ items: articles.slice(0, 2), total: 2 })
  api.getArticle = async slug => articles.find(article => article.slug === slug)
  api.getAnnotations = async () => []
  api.markRead = async () => ({})
  api.streamExplanation = (input, event, signal) => new Promise((resolve, reject) => {
    calls.push({ input, event, signal, resolve })
    signal.addEventListener('abort', () => reject(new DOMException('Stopped', 'AbortError')), { once: true })
  })
  const app = createApp(App); app.mount('#app')
  try {
    await flush()
    document.querySelector('.mobile-navigation button:last-child').click(); await flush()
    fill(document.querySelector('.ai-panel textarea'), '主窗口的问题')
    await nextTick(); document.querySelector('.send-btn').click(); await flush()
    document.querySelector('.add-chat-window').click(); await flush()
    const popup = document.querySelector('.floating-chat')
    assert.ok(popup)
    const popupId = popup.dataset.chatWindow
    const input = popup.querySelector('textarea')
    fill(input, '接着上面的答案举例')
    input.setSelectionRange(input.value.length, input.value.length)
    input.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, bubbles: true, cancelable: true }))
    await nextTick()
    assert.ok(input.value.endsWith('\n'))
    assert.equal(calls.length, 1, 'Ctrl+Enter adds a newline without sending')
    const audience = popup.querySelector('[aria-label="子窗口学习程度"]')
    audience.value = '小学生'; audience.dispatchEvent(new Event('change', { bubbles: true }))
    input.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true }))
    await flush()
    assert.match(popup.querySelector('.stream-status').textContent, /排队/)
    assert.equal(calls.length, 1)
    calls[0].event({ type: 'done', explanation: answer('第一轮解释') }); calls[0].resolve(); await flush()
    assert.equal(calls.length, 2)
    assert.equal(calls[1].input.history[1].content, '第一轮解释')
    assert.equal(calls[1].input.audience, '小学生')
    calls[1].event({ type: 'done', explanation: answer('苹果的例子') }); calls[1].resolve(); await flush()
    assert.match(popup.textContent, /苹果的例子/)
    assert.doesNotMatch(document.querySelector('.ai-panel .conversation-thread').textContent, /苹果的例子/)
    assert.match(document.querySelector('.chat-session-bar').textContent, /2 \/ 6/)
    fill(input, '未发送的草稿'); await nextTick()
    popup.querySelector('[aria-label^="收起"]').click(); await flush()
    assert.equal(popup.style.display, 'none')
    document.querySelector('.chat-window-dock button').click(); await flush()
    assert.equal(popup.querySelector('textarea').value, '未发送的草稿')
    popup.querySelector('[aria-label^="关闭"]').click(); await flush()
    assert.equal(document.querySelector(`[data-chat-window="${popupId}"]`), null)
    fill(document.querySelector('.ai-panel textarea'), '接着苹果例子追问'); await nextTick()
    document.querySelector('.send-btn').click(); await flush()
    assert.equal(calls[2].input.history[3].content, '苹果的例子', 'closed window remains in shared history')
    calls[2].event({ type: 'done', explanation: answer('后续解释') }); calls[2].resolve(); await flush()
    document.querySelector('.add-chat-window').click(); await flush()
    window.confirm = () => false
    document.querySelector('.conversation-toolbar button').click(); await flush()
    assert.match(document.querySelector('.chat-session-bar').textContent, /3 \/ 6/)
    window.confirm = () => true
    document.querySelector('.conversation-toolbar button').click(); await flush()
    assert.equal(document.querySelector('.conversation-turn'), null)
    assert.match(document.querySelector('.chat-session-bar').textContent, /0 \/ 6/)
    assert.equal(document.querySelector('.floating-chat textarea').value, '')
    document.querySelectorAll('.article-card')[1].click(); await flush()
    assert.equal(document.querySelector('.floating-chat-topic').textContent, articles[1].title)
  } finally { app.unmount(); Object.assign(api, saved) }
})
