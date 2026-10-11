import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { JSDOM } from 'jsdom'
import { parse, compileScript } from 'vue/compiler-sfc'
import ts from 'typescript'

const dom = new JSDOM('<meta name="theme-color"><div id="app"></div>', { url: 'http://localhost:5173/' })
for (const name of ['window', 'document', 'Document', 'Element', 'HTMLElement', 'SVGElement', 'Node', 'Event', 'DOMException', 'localStorage']) globalThis[name] = dom.window[name]
dom.window.HTMLElement.prototype.scrollTo = () => {}
window.matchMedia = () => ({ matches: true })
const { createApp, nextTick } = await import('vue')
const { api } = await import('../src/api.ts')
const { articles } = await import('../src/mock.ts')
const { descriptor } = parse(readFileSync(new URL('../src/App.vue', import.meta.url), 'utf8'))
let source = compileScript(descriptor, { id: 'workspace-test', inlineTemplate: true }).content
source = source.replace(/import (\w+) from ['"]\.\/components\/[^'"]+\.vue['"]/g, 'const $1 = { render: () => null }')
  .replace(/from ['"]vue['"]/g, `from ${JSON.stringify(import.meta.resolve('vue'))}`)
  .replace(/from ['"]\.\/([^'"]+)['"]/g, (_, path) => `from ${JSON.stringify(new URL(`../src/${path}.ts`, import.meta.url).href)}`)
const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ESNext } }).outputText
const App = (await import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`)).default
const flush = async (ms = 0) => { await new Promise(resolve => setTimeout(resolve, ms)); await nextTick() }
const nav = text => [...document.querySelectorAll('.mobile-navigation button')].find(el => el.textContent === text)
function search(text) {
  const input = document.querySelector('.global-search input')
  input.value = text
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

test('search, empty results and filtering preserve reading and AI conversation; only a clicked result switches the article', async () => {
  const saved = { ...api }
  const details = []
  let listCalls = 0
  api.me = async () => ({ user: { id: 1, name: '学习者', role: 'learner' } })
  api.getStats = async () => ({ articleCount: 2, sectionCount: 5 })
  api.getCategories = async () => []
  api.getCapabilities = async () => ({ aiConfigured: true, speechConfigured: false })
  api.listArticles = async ({ q }) => {
    listCalls++
    const items = q === '无结果' ? [] : q ? [articles[1]] : articles.slice(0, 2)
    return { items, total: items.length }
  }
  api.getArticle = async slug => { details.push(slug); return articles.find(a => a.slug === slug) }
  api.getAnnotations = async () => []
  api.markRead = async () => ({})
  api.streamExplanation = async (_, event) => event({ type: 'done', explanation: { mode: 'model', summary: '保留这个回答', steps: [], citations: [], checks: [], confidence: .8 } })
  const app = createApp(App)
  app.mount('#app')
  try {
    await flush(20)
    assert.deepEqual(details, [articles[0].slug], 'initial article detail loads only once')
    assert.ok(document.querySelector('.mobile-results'))
    nav('AI / 批注').click()
    document.querySelector('.send-btn').click()
    await flush()
    assert.match(document.querySelector('.ai-panel').textContent, /保留这个回答/)
    search('梯度')
    await flush(260)
    assert.equal(document.querySelector('.article-body h1').textContent, articles[0].title)
    assert.match(document.querySelector('.ai-panel').textContent, /保留这个回答/)
    assert.deepEqual(details, [articles[0].slug])
    assert.ok(document.querySelector('.mobile-results'))
    search('无结果')
    await flush(260)
    assert.match(document.querySelector('.item-list').textContent, /没有找到匹配/)
    assert.equal(document.querySelector('.article-body h1').textContent, articles[0].title)
    nav('正文阅读').click()
    await nextTick()
    assert.ok(document.querySelector('.mobile-reader'))
    search('梯度')
    await flush(260)
    document.querySelector('.article-card').click()
    await flush(20)
    assert.ok(document.querySelector('.mobile-reader'))
    assert.equal(document.querySelector('.article-body h1').textContent, articles[1].title)
    assert.deepEqual(details, [articles[0].slug, articles[1].slug])
    assert.doesNotMatch(document.querySelector('.ai-panel').textContent, /保留这个回答/)
    nav('AI / 批注').click()
    await nextTick()
    assert.ok(document.querySelector('.mobile-insight'))
    document.querySelector('.close-panel').click()
    await nextTick()
    assert.ok(document.querySelector('.mobile-reader'))
    document.querySelector('[aria-label="显示或隐藏右侧面板"]').click()
    await nextTick()
    assert.ok(document.querySelector('.mobile-insight'))
    assert.equal(listCalls, 4)
  } finally { app.unmount(); Object.assign(api, saved) }
})
