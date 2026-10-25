import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { JSDOM } from 'jsdom'
import { parse, compileScript } from 'vue/compiler-sfc'
import ts from 'typescript'

const dom = new JSDOM('<!doctype html><meta name="theme-color"><div id="app"></div>', { url: 'http://localhost/', pretendToBeVisual: true })
for (const name of ['window', 'document', 'Document', 'Element', 'HTMLElement', 'HTMLImageElement', 'HTMLDetailsElement', 'SVGElement', 'Node', 'Event', 'DOMException', 'localStorage']) globalThis[name] = dom.window[name]
window.matchMedia = () => ({ matches: true })
globalThis.requestAnimationFrame = window.requestAnimationFrame.bind(window)
globalThis.cancelAnimationFrame = window.cancelAnimationFrame.bind(window)
globalThis.getComputedStyle = window.getComputedStyle.bind(window)
window.confirm = () => true
let scrolled
HTMLElement.prototype.scrollTo = () => {}
HTMLElement.prototype.scrollIntoView = function () { scrolled = this }
dom.window.Range.prototype.getBoundingClientRect = () => ({ left: 10, top: 120, width: 100 })
const { createApp, nextTick } = await import('vue')
const { api } = await import('../src/api.ts')
const { articles } = await import('../src/mock.ts')

function compile(file, transform = source => source) {
  const url = new URL(file, import.meta.url)
  const { descriptor } = parse(readFileSync(url, 'utf8'))
  let source = transform(compileScript(descriptor, { id: file, inlineTemplate: true }).content)
  source = source.replace(/import ['"][^'"]+\.css['"]/g, '')
    .replace(/from ['"]([^'"]+)['"]/g, (_, specifier) => `from ${JSON.stringify(specifier.startsWith('.') ? new URL(specifier + '.ts', url).href : import.meta.resolve(specifier))}`)
  const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ESNext } }).outputText
  return `data:text/javascript;base64,${Buffer.from(js).toString('base64')}`
}
const wikiURL = compile('../src/components/WikiContent.vue')
const appURL = compile('../src/App.vue', source => source
  .replace(/import WikiContent from ['"]\.\/components\/WikiContent.vue['"]/, `import WikiContent from '${wikiURL}'`)
  .replace(/import (\w+) from ['"]\.\/components\/[^'"]+\.vue['"]/g, 'const $1 = { render: () => null }'))
const App = (await import(appURL)).default
const flush = async () => { await new Promise(resolve => setTimeout(resolve, 15)); await nextTick() }

test('select formatted text, publish, reload, locate on mobile and delete its highlight', async () => {
  const saved = { ...api }
  const article = { ...articles[0], sections: [
    { id: 101, title: '说明', content: '第一处：边权非负。\n第二处：边权非负。', contentHtml: '<p>第一处：边权非负。</p><p>第二处：<strong>边权</strong>非负。</p>' },
    { id: 102, title: '其他章节', content: '边权非负。' }
  ] }
  let notes = [], submitted
  api.me = async () => ({ user: { id: 1, name: '作者', role: 'admin' } })
  api.getStats = async () => ({ articleCount: 1, sectionCount: 2 })
  api.getCategories = async () => []
  api.getCapabilities = async () => ({ aiConfigured: false, speechConfigured: false })
  api.listArticles = async () => ({ items: [article], total: 1 })
  api.getArticle = async () => article
  api.getAnnotations = async () => structuredClone(notes)
  api.markRead = async () => ({})
  api.createAnnotation = async input => {
    submitted = input
    const note = { ...input, id: 9, userId: 1, author: '作者', status: 'visible', createdAt: '2026-10-25', replies: 0, likes: 0 }
    notes.push(note)
    return structuredClone(note)
  }
  api.deleteAnnotation = async id => { notes = notes.filter(note => note.id !== id); return {} }
  let app = createApp(App)
  app.mount('#app')
  try {
    await flush()
    document.querySelector('.article-card').click()
    await flush()
    const strong = document.querySelector('#section-101 strong')
    const range = document.createRange()
    range.setStart(strong.firstChild, 0)
    range.setEnd(strong.nextSibling, 2)
    window.getSelection().addRange(range)
    document.dispatchEvent(new Event('selectionchange'))
    await nextTick()
    const annotate = [...document.querySelectorAll('.selection-toolbar button')].find(button => button.textContent.includes('批注'))
    assert.ok(annotate)
    annotate.click()
    await flush()
    assert.ok(document.querySelector('mark.is-draft'), 'draft remains highlighted after focus leaves the article')
    const input = document.querySelector('.annotation-input')
    input.value = '这里需要非负边权'
    input.dispatchEvent(new Event('input', { bubbles: true }))
    await nextTick()
    document.querySelector('.note-composer .primary-btn').click()
    await flush()
    assert.equal(submitted.quote, '边权非负')
    assert.equal(submitted.sectionId, 101)
    assert.ok(submitted.prefix.includes('第二处：'))
    assert.equal(document.querySelector('#section-102 mark'), null, 'same text in another section is untouched')
    assert.equal(document.querySelector('#section-101 p:first-child mark'), null, 'first occurrence is untouched')
    assert.equal(document.querySelector('mark.is-draft'), null)
    app.unmount()
    app = createApp(App); app.mount('#app')
    await flush()
    assert.equal(document.querySelectorAll('#section-101 mark').length, 2, 'anchors survive a fresh API load')
    // Use the tab by its visible text, independent of layout class names.
    const notesTab = [...document.querySelectorAll('.insight-pane button')].find(b => /^批注/.test(b.textContent.trim()))
    notesTab.click(); await flush()
    document.querySelector('.quote-strip').click(); await flush()
    assert.ok(document.querySelector('.mobile-reader'))
    assert.equal(scrolled.dataset.annotationIds, '9')
    assert.equal(document.querySelectorAll('mark.is-active').length, 2)
    document.querySelector('mark').click(); await flush()
    assert.ok(document.querySelector('.mobile-insight'))
    assert.ok(document.querySelector('#annotation-9.is-selected'))
    const remove = [...document.querySelectorAll('#annotation-9 button')].find(b => b.textContent === '删除批注')
    remove.click(); await flush()
    assert.equal(document.querySelector('.article-body mark'), null)
  } finally { window.getSelection().removeAllRanges(); app.unmount(); Object.assign(api, saved) }
})
