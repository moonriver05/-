import test from 'node:test'
import assert from 'node:assert/strict'
import { JSDOM } from 'jsdom'
import { handleComposerKey } from '../src/explanationStream.ts'

test('Enter sends; Ctrl+Enter inserts a newline at the selection and updates input', () => {
  const dom = new JSDOM('<textarea>前后</textarea>')
  const previous = globalThis.Event
  globalThis.Event = dom.window.Event
  try {
    const input = dom.window.document.querySelector('textarea')
    let sent = 0, updated = 0
    input.addEventListener('keydown', event => handleComposerKey(event, () => sent++))
    input.addEventListener('input', () => updated++)
    input.setSelectionRange(1, 1)
    const enter = new dom.window.KeyboardEvent('keydown', { key: 'Enter', cancelable: true })
    input.dispatchEvent(enter)
    assert.equal(sent, 1)
    assert.equal(enter.defaultPrevented, true)
    input.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Enter', ctrlKey: true, cancelable: true }))
    assert.equal(input.value, '前\n后')
    assert.equal(input.selectionStart, 2)
    assert.equal(updated, 1)
    assert.equal(sent, 1)
    input.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Enter', isComposing: true, cancelable: true }))
    input.dispatchEvent(new dom.window.KeyboardEvent('keydown', { key: 'Enter', repeat: true, cancelable: true }))
    assert.equal(sent, 1)
  } finally { globalThis.Event = previous; dom.window.close() }
})
