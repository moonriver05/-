import test from 'node:test'
import assert from 'node:assert/strict'
import { api, ApiError } from '../src/api.ts'

test('stream endpoint preserves Retry-After for the shared queue and does not retry the request', async () => {
  const savedFetch = globalThis.fetch, savedWindow = globalThis.window
  let calls = 0
  globalThis.window = { setTimeout, clearTimeout, dispatchEvent() {} }
  globalThis.fetch = async () => {
    calls++
    return new Response(JSON.stringify({ error: '提问过于频繁' }), { status: 429, headers: { 'Content-Type': 'application/json', 'Retry-After': '23' } })
  }
  try {
    await assert.rejects(api.streamExplanation({ articleId: 1, question: '解释', audience: '本科生', style: '直觉' }, () => {}, new AbortController().signal), error => error instanceof ApiError && error.status === 429 && error.retryAfter === 23)
    assert.equal(calls, 1)
  } finally { globalThis.fetch = savedFetch; globalThis.window = savedWindow }
})
