import assert from 'node:assert/strict'
import { test } from 'node:test'
import { JSDOM } from 'jsdom'
import katex from 'katex'

const dom = new JSDOM('', { url: 'http://localhost:5173/' })
globalThis.window = dom.window
globalThis.document = dom.window.document
const { safeWikiHTML, wikiFilename } = await import('../src/wikiMedia.ts')
const image = '<figure><img src="https://commons.wikimedia.org/wiki/Special:FilePath/X.png?width=720" alt="图"><figcaption>原图</figcaption></figure>'
const parsed = value => new JSDOM(value).window.document

test('external images connect directly to Wikimedia, not the app server', () => {
  const doc = parsed(safeWikiHTML(image, {}, 'external'))
  const img = doc.querySelector('img')
  assert.equal(img.getAttribute('src'), 'https://commons.wikimedia.org/wiki/Special:FilePath/X.png?width=720')
  assert.equal(img.getAttribute('loading'), 'lazy')
})

test('mirror-only and off never fall back to the remote original', () => {
  for (const mode of ['mirror-only', 'off']) {
    const doc = parsed(safeWikiHTML(image, {}, mode))
    assert.equal(doc.querySelector('img'), null)
    assert.equal(doc.querySelector('figcaption').textContent, '原图')
  }
})

test('uncaptioned image filenames do not clutter article tables', () => {
  const html = '<figure><img src="https://commons.wikimedia.org/wiki/Special:FilePath/X.png" alt="X.png"><figcaption>X.png · <a href="https://commons.wikimedia.org/wiki/File:X.png">图源与授权</a></figcaption></figure>'
  const doc = parsed(safeWikiHTML(html, {}, 'external'))
  assert.equal(doc.querySelector('figcaption').textContent, '图源与授权')
  assert.equal(doc.querySelector('img').alt, 'X.png')
})

test('CDN manifest replaces the original URL and includes attribution', () => {
  const doc = parsed(safeWikiHTML(image, { 'X.png': { url: 'https://cdn.example.com/wiki/x.webp', author: 'Test author', license: 'CC BY-SA 4.0', licenseUrl: 'https://creativecommons.org/licenses/by-sa/4.0/', sourceUrl: 'https://commons.wikimedia.org/wiki/File:X.png' } }, 'mirror-only'))
  assert.equal(doc.querySelector('img').src, 'https://cdn.example.com/wiki/x.webp')
  assert.match(doc.querySelector('figcaption').textContent, /Test author.*CC BY-SA 4.0/)
})

test('untrusted image URLs, event handlers and script links are removed before display', () => {
  const unsafe = '<script>alert(1)</script><img src="https://evil.example/track" onerror="bad()"><a href="javascript:alert(1)">bad</a><iframe src="http://127.0.0.1:3306"></iframe>'
  const doc = parsed(safeWikiHTML(unsafe))
  assert.equal(doc.querySelector('script,iframe,img,[onerror],a[href]'), null)
  assert.equal(wikiFilename('https://commons.wikimedia.org.evil.example/wiki/Special:FilePath/X.png'), null)
  assert.equal(wikiFilename('https://commons.wikimedia.org/wiki/Special:FilePath/A%20B.svg'), 'A_B.svg')
})

test('math and table structure survive sanitization; TeX cannot create active links', () => {
  const doc = parsed(safeWikiHTML('<table><tr><td colspan="2">值</td></tr></table><span class="wiki-math" data-tex="x^2" data-display="true">x^2</span>'))
  assert.equal(doc.querySelector('td').colSpan, 2)
  assert.equal(doc.querySelector('[data-tex]').dataset.tex, 'x^2')
  const output = katex.renderToString('\\href{javascript:alert(1)}{x}', { trust: false, throwOnError: false })
  assert.equal(parsed(output).querySelector('a'), null)
})
