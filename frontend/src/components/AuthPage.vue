<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { api, ApiError } from '../api'
import type { User } from '../types'
const emit = defineEmits<{ close: []; authenticated: [user: User] }>()
const mode = ref<'login' | 'register'>('login')
const username = ref(''), name = ref(''), password = ref(''), confirmation = ref('')
const busy = ref(false), error = ref('')
const email = ref(''), code = ref(''), sending = ref(false), mailNotice = ref('')
const mailConfigured = ref<boolean | null>(null)
const now = ref(Date.now()), resendAt = ref(0)
const cooldown = computed(() => Math.max(0, Math.ceil((resendAt.value - now.value) / 1000)))
const emailInput = ref<HTMLInputElement | null>(null)
let timer: number | undefined
watch(email, () => { code.value = ''; mailNotice.value = '' })
const usernameInput = ref<HTMLInputElement | null>(null)
onMounted(async () => {
  usernameInput.value?.focus()
  timer = window.setInterval(() => { now.value = Date.now() }, 1000)
  try { mailConfigured.value = (await api.authConfig()).emailConfigured } catch { mailConfigured.value = null }
})
onUnmounted(() => window.clearInterval(timer))
async function sendCode() {
  if (sending.value || busy.value || cooldown.value > 0) return
  if (!emailInput.value?.reportValidity()) return
  error.value = ''; mailNotice.value = ''; sending.value = true
  const destination = email.value.trim()
  try {
    const result = await api.sendEmailCode(destination)
    now.value = Date.now(); resendAt.value = now.value + result.retryAfter * 1000
    if (destination === email.value.trim()) mailNotice.value = result.message + '（10 分钟内有效）'
  } catch (e) {
    error.value = e instanceof Error ? e.message : '验证码发送失败，请稍后重试'
    if (e instanceof ApiError && e.retryAfter > 0) { now.value = Date.now(); resendAt.value = now.value + e.retryAfter * 1000 }
  }
  finally { sending.value = false }
}
async function submit() {
  if (busy.value || sending.value) return
  error.value = ''
  if (mode.value === 'register' && password.value !== confirmation.value) { error.value = '两次输入的密码不一致'; return }
  busy.value = true
  try {
    const result = mode.value === 'login' ? await api.login({ username: username.value, password: password.value }) : await api.register({ username: username.value, name: name.value, password: password.value, email: email.value.trim(), code: code.value })
    password.value = ''; confirmation.value = ''; emit('authenticated', result.user)
  } catch (e) { error.value = e instanceof Error ? e.message : '暂时无法登录，请稍后重试' }
  finally { busy.value = false }
}
</script>

<template>
  <main class="auth-page">
    <button class="text-btn auth-back" :disabled="busy" @click="emit('close')">← 返回知识库</button>
    <section class="auth-card" :class="{ 'auth-register': mode === 'register' }">
      <div class="brand-mark">∑</div>
      <p class="auth-eyebrow">MathWiki LAB</p>
      <h1>{{ mode === 'login' ? '欢迎回来' : '建立你的学习账户' }}</h1>
      <p class="auth-description">收藏知识，记录思考，与他人一起理解数学。</p>
      <form @submit.prevent="submit">
        <label>账号<input ref="usernameInput" v-model="username" required autocomplete="username" placeholder="字母、数字或下划线" pattern="[A-Za-z0-9_]{3,40}" maxlength="40" /></label>
        <label v-if="mode === 'register'">昵称<input v-model="name" required autocomplete="nickname" maxlength="40" placeholder="希望大家如何称呼你" /></label>
        <template v-if="mode === 'register'">
          <label>邮箱<input ref="emailInput" v-model="email" type="email" required autocomplete="email" maxlength="254" placeholder="用于接收注册验证码" :disabled="sending || busy" /></label>
          <label for="registration-code">邮箱验证码</label>
          <div class="email-code-row"><input id="registration-code" v-model="code" required inputmode="numeric" autocomplete="one-time-code" pattern="[0-9]{6}" maxlength="6" placeholder="6 位验证码" /><button type="button" class="email-code-send" :disabled="sending || busy || cooldown > 0 || !email.trim() || mailConfigured === false" @click="sendCode">{{ sending ? '发送中…' : cooldown > 0 ? `${cooldown} 秒后重发` : '发送验证码' }}</button></div>
          <p v-if="mailConfigured === false" class="email-notice" role="status">发信服务尚未配置，暂时无法注册。已有账户仍可登录。</p>
          <p v-if="mailNotice" class="email-notice" role="status">{{ mailNotice }}</p>
        </template>
        <label>密码<input v-model="password" type="password" required :autocomplete="mode === 'login' ? 'current-password' : 'new-password'" :minlength="mode === 'register' ? 8 : undefined" maxlength="72" placeholder="至少 8 个字符" /></label>
        <label v-if="mode === 'register'">确认密码<input v-model="confirmation" type="password" required autocomplete="new-password" maxlength="72" placeholder="再次输入密码" /></label>
        <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
        <button class="primary-btn auth-submit" :disabled="busy || sending || (mode === 'register' && mailConfigured === false)">{{ busy ? '请稍候…' : mode === 'login' ? '登录' : '验证并创建账户' }}</button>
      </form>
      <p class="auth-switch">{{ mode === 'login' ? '还没有账户？' : '已经有账户？' }} <button class="text-btn" :disabled="busy" @click="mode = mode === 'login' ? 'register' : 'login'; error = ''; password = ''; confirmation = ''">{{ mode === 'login' ? '注册' : '去登录' }}</button></p>
      <small>无需登录也能阅读。新账户默认为普通用户。</small>
    </section>
  </main>
</template>
