import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'
import { renderAIText, safeSourceURL } from '../src/aiText.ts'
import { answerBadge, groundingNotice, webSearchNotice } from '../src/answerStatus.ts'
const parsed = html => new JSDOM(html).window.document

test('renders inline and block formulas; incomplete streams and invalid TeX remain readable', () => {
  const doc = parsed(renderAIText('链式法则：$\\frac{dL}{dx}=\\frac{dL}{dy}\\frac{dy}{dx}$\n$$y_i^{l}=f^{l}(z_i^{l})$$'))
  assert.equal(doc.querySelectorAll('.katex').length, 2)
  assert.equal(doc.querySelectorAll('.katex-display').length, 1)
  assert.match(parsed(renderAIText('未完成 $x_{')).body.textContent, /\$x_\{/)
  assert.ok(parsed(renderAIText('$\\notacommand{x}$')).querySelector('.ai-math-fallback'))
})

test('model HTML and math links cannot become active content; code stays literal', () => {
  const doc = parsed(renderAIText('<img src=x onerror=alert(1)><script>alert(1)</script> $\\href{javascript:alert(1)}{x}$ `x_$`\n```tex\n$not_math$\n```'))
  assert.equal(doc.querySelector('img,script,iframe,a[href]'), null)
  assert.match(doc.body.textContent, /<script>/)
  assert.match(doc.querySelector('pre code').textContent, /\$not_math\$/)
})

test('formula processing is bounded and external links reject unsafe schemes', () => {
  assert.ok(parsed(renderAIText(Array(70).fill('$x$').join(' '))).querySelectorAll('.katex').length <= 48)
  assert.equal(safeSourceURL('javascript:alert(1)'), undefined)
  assert.equal(safeSourceURL('https://name:password@example.com'), undefined)
  assert.equal(safeSourceURL('https://example.edu/math'), 'https://example.edu/math')
})

test('uncited model explanations and failed search are never presented as verified sources', () => {
  const answer = { mode: 'model', answerMode: 'balanced', grounding: 'model', webSearch: { status: 'failed', count: 0 } }
  assert.match(answerBadge(answer), /暂无可核查引文/)
  assert.match(groundingNotice(answer), /模型推导/)
  assert.match(webSearchNotice(answer), /暂不可用/)
})
