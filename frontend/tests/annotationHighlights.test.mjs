import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'
import { anchorAt, captureAnchor, locateQuote, paintHighlights, clearHighlights } from '../src/annotationHighlights.ts'

function content(html) {
  return new JSDOM(`<article>${html}</article>`).window.document.querySelector('article')
}

test('repeated quotes use offsets and context; ambiguous or removed text is not guessed', () => {
  const text = '第一处：非负边权；第二处：非负边权。'
  const start = text.lastIndexOf('非负边权')
  const anchor = anchorAt(text, start, start + 4)
  assert.deepEqual(locateQuote(text, anchor), { start, end: start + 4 })
  assert.equal(locateQuote(text, { quote: '非负边权' }), null)
  assert.deepEqual(locateQuote('新增。' + text, anchor), { start: start + 3, end: start + 7 })
  assert.equal(locateQuote('已改为其他内容', anchor), null)
})

test('selection across emphasis and links persists exactly, including emoji UTF-16 offsets', () => {
  const root = content('<p>🙂开始<strong>非负</strong><a href="https://example.com">边权</a>结束</p>')
  const range = root.ownerDocument.createRange()
  range.setStart(root.querySelector('strong').firstChild, 0)
  range.setEnd(root.querySelector('a').firstChild, 2)
  const anchor = captureAnchor(root, range)
  assert.equal(anchor.quote, '非负边权')
  assert.equal(anchor.startOffset, 4)
  paintHighlights(root, [{ id: 7, ...anchor }], 7)
  assert.equal([...root.querySelectorAll('mark')].map(n => n.textContent).join(''), '非负边权')
  assert.equal(root.querySelectorAll('mark.is-active').length, 2)
  assert.equal(root.querySelector('a').getAttribute('href'), 'https://example.com')
  assert.equal(root.textContent, '🙂开始非负边权结束')
  paintHighlights(root, [{ id: 7, ...anchor }])
  assert.equal(root.querySelectorAll('mark mark').length, 0)
  assert.equal(root.querySelectorAll('mark.is-active').length, 0)
  clearHighlights(root)
  assert.equal(root.querySelectorAll('mark').length, 0)
  assert.equal(root.querySelector('strong').textContent, '非负')
})

test('overlapping notes share a highlight; deleting one preserves the other and deleting both clears it', () => {
  const root = content('<p>甲乙丙丁戊</p>')
  const first = { id: 1, quote: '甲乙丙' }, second = { id: 2, quote: '乙丙丁' }
  paintHighlights(root, [first, second], 2)
  assert.equal(root.querySelector('[data-annotation-ids="1 2"]').textContent, '乙丙')
  paintHighlights(root, [first])
  assert.equal(root.querySelector('mark').textContent, '甲乙丙')
  assert.equal(root.querySelector('[data-annotation-ids~="2"]'), null)
  paintHighlights(root, [])
  assert.equal(root.innerHTML, '<p>甲乙丙丁戊</p>')
})

test('math markup is untouched and selections crossing roots or formulas are rejected', () => {
  const root = content('<p>公式<span data-tex="x">x</span>结束</p>')
  paintHighlights(root, [{ id: 1, quote: '公式x' }])
  assert.equal(root.querySelector('mark'), null)
  const range = root.ownerDocument.createRange()
  range.selectNodeContents(root)
  assert.equal(captureAnchor(root, range), null)
  const external = root.ownerDocument.createTextNode('外部文字')
  root.after(external)
  range.setStart(root.querySelector('p').firstChild, 0)
  range.setEnd(external, 2)
  assert.equal(captureAnchor(root, range), null)
})

test('quote strings are treated as text, never injected as HTML', () => {
  const root = content('<p>&lt;img src=x onerror=alert(1)&gt;</p>')
  paintHighlights(root, [{ id: 1, quote: '<img src=x onerror=alert(1)>' }])
  assert.equal(root.querySelector('img'), null)
  assert.equal(root.querySelector('mark').textContent, '<img src=x onerror=alert(1)>')
})
