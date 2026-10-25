import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { JSDOM } from 'jsdom'
import { parse, compileScript } from 'vue/compiler-sfc'
import ts from 'typescript'

const dom = new JSDOM('<div id="app"></div>', { url: 'http://localhost:5173/' })
for (const name of ['window', 'document', 'Document', 'Element', 'HTMLElement', 'SVGElement', 'Node', 'Event', 'DOMException']) globalThis[name] = dom.window[name]
const { createApp, nextTick } = await import('vue')
const { api } = await import('../src/api.ts')
async function component(name) {
  const url = new URL(`../src/components/${name}.vue`, import.meta.url)
  const { descriptor } = parse(readFileSync(url, 'utf8'), { filename: url.pathname })
  let source = compileScript(descriptor, { id: name, inlineTemplate: true }).content
  source = source.replace(/from ['"]vue['"]/g, `from ${JSON.stringify(import.meta.resolve('vue'))}`)
    .replace(/from ['"]\.\.\/api['"]/g, `from ${JSON.stringify(new URL('../src/api.ts', import.meta.url).href)}`)
  const js = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ESNext } }).outputText
  return (await import(`data:text/javascript;base64,${Buffer.from(js).toString('base64')}`)).default
}
const AuthPage = await component('AuthPage'), AccountManagement = await component('AccountManagement')
const flush = async () => { await new Promise(resolve => setTimeout(resolve, 0)); await nextTick() }
const button = text => [...document.querySelectorAll('button')].find(el => el.textContent.includes(text))
function input(el, value) { assert.ok(el); el.value = value; el.dispatchEvent(new Event('input', { bubbles: true })) }

test('forgot-password UI sends reset code and returns to login without auto-login', async () => {
  const saved = { ...api }; const calls = []
  api.authConfig = async () => ({ emailConfigured: true })
  api.sendResetCode = async email => { calls.push(['code', email]); return { message: '如果邮箱已绑定将发送验证码', retryAfter: 60 } }
  api.resetPassword = async (...args) => { calls.push(['reset', ...args]); return { message: '密码已重置，请重新登录' } }
  let authenticated = false
  const app = createApp(AuthPage, { onAuthenticated: () => { authenticated = true } }); app.mount('#app')
  try {
    await flush(); button('忘记密码').click(); await flush()
    assert.match(document.querySelector('h1').textContent, /找回/)
    input(document.querySelector('input[type=email]'), 'learner@example.com'); await flush()
    button('发送验证码').click(); await flush()
    assert.deepEqual(calls[0], ['code', 'learner@example.com'])
    input(document.querySelector('input[inputmode=numeric]'), '123456')
    for (const el of document.querySelectorAll('input[type=password]')) input(el, 'New-test-password')
    await nextTick(); document.querySelector('form').dispatchEvent(new Event('submit', { bubbles: true, cancelable: true })); await flush()
    assert.deepEqual(calls[1], ['reset', 'learner@example.com', '123456', 'New-test-password'])
    assert.match(document.querySelector('h1').textContent, /欢迎/)
    assert.match(document.body.textContent, /密码已重置/); assert.equal(authenticated, false)
  } finally { app.unmount(); Object.assign(api, saved) }
})

test('administrator UI requires a reason and refreshes ban state', async () => {
  const saved = { ...api }; let banned = false; const calls = []
  api.adminUsers = async () => ({ items: [{ id: 2, username: 'learner', name: '用户', role: 'learner', banned }, { id: 1, username: 'admin', name: '管理员', role: 'admin' }], total: 2 })
  api.banUser = async (...args) => { calls.push(args); banned = args[1] }
  const app = createApp(AccountManagement, { mode: 'users' }); app.mount('#app')
  try {
    await flush(); window.prompt = () => '   '; button('封禁账户').click(); await flush(); assert.equal(calls.length, 0)
    window.prompt = () => '重复垃圾评论'; button('封禁账户').click(); await flush()
    assert.deepEqual(calls[0], [2, true, '重复垃圾评论']); assert.ok(button('解除封禁'))
    window.prompt = () => '复核通过'; button('解除封禁').click(); await flush()
    assert.deepEqual(calls[1], [2, false, '复核通过'])
    assert.equal([...document.querySelectorAll('button')].filter(el => el.textContent === '封禁账户').length, 1)
  } finally { app.unmount(); Object.assign(api, saved) }
})

test('role assignment needs a reason and confirmation, hides self and banned targets, then refreshes', async () => {
  const saved = { ...api }; const calls = []; let role = 'learner'
  const prompt = window.prompt, confirm = window.confirm
  api.adminUsers = async () => ({ items: [
    { id: 2, username: 'helper', name: '协助者', role },
    { id: 1, username: 'admin', name: '自己', role: 'admin' },
    { id: 3, username: 'banned', name: '已封禁者', role: 'learner', banned: true },
  ], total: 3 })
  api.setUserRole = async (...args) => { calls.push(args); role = args[1]; return { ok: true, changed: true, message: '成功' } }
  const app = createApp(AccountManagement, { mode: 'users', currentUserId: 1 }); app.mount('#app')
  try {
    await flush()
    assert.equal([...document.querySelectorAll('button')].filter(b => b.textContent === '授予管理员').length, 1)
    assert.equal(button('撤销管理员'), undefined)
    window.prompt = () => ' '; window.confirm = () => true
    button('授予管理员').click(); await flush(); assert.equal(calls.length, 0)
    window.prompt = () => '协助审核'; window.confirm = () => false
    button('授予管理员').click(); await flush(); assert.equal(calls.length, 0)
    window.confirm = () => true
    button('授予管理员').click(); await flush()
    assert.deepEqual(calls[0], [2, 'admin', '协助审核'])
    assert.match(document.body.textContent, /重新登录/)
    assert.equal([...document.querySelectorAll('button')].filter(b => b.textContent === '撤销管理员').length, 1)
    button('撤销管理员').click(); await flush()
    assert.deepEqual(calls[1], [2, 'learner', '协助审核'])
    assert.ok(button('授予管理员'))
  } finally { app.unmount(); Object.assign(api, saved); window.prompt = prompt; window.confirm = confirm }
})
