<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { api } from '../api'
import type { User } from '../types'
const props = defineProps<{ mode: 'users' | 'audits'; currentUserId?: number }>()
const users = ref<User[]>([]), audits = ref<Awaited<ReturnType<typeof api.adminAudits>>['items']>([])
const page = ref(1), total = ref(0), query = ref(''), busy = ref(false), error = ref('')
const notice = ref('')
let disposed = false
onMounted(load)
onUnmounted(() => { disposed = true })
async function load() {
  busy.value = true; error.value = ''
  try {
    if (props.mode === 'users') { const data = await api.adminUsers(page.value, query.value.trim()); if (disposed) return; users.value = data.items; total.value = data.total }
    else { const data = await api.adminAudits(page.value); if (disposed) return; audits.value = data.items; total.value = data.total }
  } catch (e) { if (!disposed) error.value = e instanceof Error ? e.message : '加载失败' }
  finally { if (!disposed) busy.value = false }
}
async function changeBan(user: User) {
  if (busy.value) return
  const reason = window.prompt(`${user.banned ? '解封' : '封禁'}「${user.name}」：请填写原因（1–200 字）。封禁会退出其所有登录。`)
  if (reason === null) return
  if (!reason.trim() || [...reason].length > 200) { error.value = '请填写 1–200 字原因'; return }
  busy.value = true; error.value = ''
  try { await api.banUser(user.id, !user.banned, reason.trim()); if (!disposed) await load() }
  catch (e) { if (!disposed) error.value = e instanceof Error ? e.message : '操作失败' }
  finally { if (!disposed) busy.value = false }
}
async function changeRole(user: User) {
  if (busy.value || user.id === props.currentUserId || user.banned) return
  const role = user.role === 'admin' ? 'learner' : 'admin'
  const operation = role === 'admin' ? '授予管理员' : '撤销管理员'
  const reason = window.prompt(`${operation}「${user.name}」(@${user.username})：请填写原因（1–200 字）。`)
  if (reason === null) return
  if (!reason.trim() || [...reason.trim()].length > 200) { error.value = '请填写 1–200 字原因'; return }
  if (!window.confirm(`${operation}「${user.name}」(#${user.id})？${role === 'admin' ? '该账户将能审核内容、管理账户及授权其他管理员。' : '该账户将失去管理权限。'}身份变化会退出该账户的所有登录。`)) return
  busy.value = true; error.value = ''; notice.value = ''
  try {
    const result = await api.setUserRole(user.id, role, reason.trim())
    if (!disposed) { await load(); notice.value = result.changed ? `${operation}成功，请通知该用户重新登录。` : '该账户已是所选身份。' }
  } catch (e) { if (!disposed) error.value = e instanceof Error ? e.message : '角色变更失败' }
  finally { if (!disposed) busy.value = false }
}
</script>
<template>
  <section aria-label="账户与操作管理">
    <p class="muted management-description">{{ mode === 'users' ? '可授权或撤销其他账户的管理员身份，需填写原因并二次确认。身份变化后需重新登录；不能修改自己，封禁账户需先解封。管理员不能直接封禁。' : '记录管理员审核、删除、恢复及账户操作。不记录密码、验证码或密钥。状态未确认的操作需核对实际数据。' }}</p>
    <form v-if="mode === 'users'" class="management-search" @submit.prevent="page = 1; load()"><input v-model="query" aria-label="查找用户" placeholder="输入完整账号或昵称" :disabled="busy" /><button class="text-btn" :disabled="busy">查找</button></form>
    <p v-if="error" class="inline-error" role="alert">{{ error }}</p>
    <p v-if="notice" class="service-notice" role="status">{{ notice }}</p>
    <p v-if="busy" class="muted" role="status">正在处理…</p>
    <template v-else>
      <p v-if="!total" class="empty-state">暂无记录。</p>
      <article v-for="user in mode === 'users' ? users : []" :key="user.id" class="moderation-note">
        <div class="community-heading"><strong>{{ user.name }}</strong><small>@{{ user.username || '未设置账号' }} · #{{ user.id }}</small></div>
        <div class="community-form-footer"><span>{{ user.role === 'admin' ? '管理员' : user.banned ? '已封禁' : '普通用户 · 正常' }}{{ user.id === currentUserId ? ' · 当前账户' : '' }}</span><div><button v-if="user.id !== currentUserId && user.username && !user.banned" class="text-btn" :class="{ danger: user.role === 'admin' }" @click="changeRole(user)">{{ user.role === 'admin' ? '撤销管理员' : '授予管理员' }}</button><button v-if="user.role !== 'admin'" class="text-btn" :class="{ danger: !user.banned }" @click="changeBan(user)">{{ user.banned ? '解除封禁' : '封禁账户' }}</button></div></div>
      </article>
      <article v-for="log in mode === 'audits' ? audits : []" :key="log.id" class="moderation-note">
        <div class="community-heading"><strong>管理员 #{{ log.userId }}</strong><small>{{ new Date(log.createdAt).toLocaleString('zh-CN') }}</small></div>
        <p>{{ log.action }}<span v-if="log.target"> · 目标 #{{ log.target }}</span></p>
        <p v-if="log.reason">原因：{{ log.reason }}</p>
        <small class="muted">{{ log.status === 0 ? '执行中或结果未确认' : log.status < 400 ? '成功' : '未完成' }}<span v-if="log.status">（{{ log.status }}）</span></small>
      </article>
    </template>
    <div class="community-form-footer"><button class="text-btn" :disabled="busy || page <= 1" @click="page--; load()">上一页</button><span>{{ page }} / {{ Math.max(1, Math.ceil(total / 50)) }} · {{ total }} 条</span><button class="text-btn" :disabled="busy || page * 50 >= total" @click="page++; load()">下一页</button><button class="text-btn" :disabled="busy" @click="load">刷新</button></div>
  </section>
</template>
