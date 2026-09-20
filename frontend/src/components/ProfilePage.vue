<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { api } from '../api'
import type { User } from '../types'
import UserAvatar from './UserAvatar.vue'
const props = defineProps<{ user: User }>()
const emit = defineEmits<{ close: []; updated: [user: User] }>()
const draft = reactive({ name: props.user.name, bio: props.user.bio || '', location: props.user.location || '', website: props.user.website || '' })
const original = ref(JSON.stringify(draft))
const dirty = computed(() => JSON.stringify(draft) !== original.value)
const busy = ref(false), selecting = ref(false), configured = ref<boolean | null>(null)
const error = ref(''), success = ref(''), preview = ref(''), selected = ref<Blob | null>(null)
const back = ref<HTMLButtonElement | null>(null)
let disposed = false
onMounted(async () => {
  back.value?.focus()
  try { configured.value = (await api.profileConfig()).avatarConfigured }
  catch (e) { error.value = e instanceof Error ? e.message : '无法获取存储配置' }
})
function clearPreview() { if (preview.value) URL.revokeObjectURL(preview.value); preview.value = ''; selected.value = null }
function beforeUnload(e: BeforeUnloadEvent) { if (dirty.value || selected.value || busy.value) e.preventDefault() }
onMounted(() => window.addEventListener('beforeunload', beforeUnload))
onUnmounted(() => { disposed = true; clearPreview(); window.removeEventListener('beforeunload', beforeUnload) })
function close() {
  if (busy.value || selecting.value) return
  if ((dirty.value || selected.value) && !window.confirm('还有未保存的修改，确定返回？')) return
  emit('close')
}
async function choose(event: Event) {
  const input = event.target as HTMLInputElement, file = input.files?.[0]; input.value = ''
  if (!file || busy.value || selecting.value) return
  error.value = ''; success.value = ''; selecting.value = true
  let rawURL = ''
  try {
    if (!['image/jpeg', 'image/png'].includes(file.type) || file.size > 5 * 1024 * 1024) throw new Error('请选择不超过 5 MB 的 JPG 或 PNG 图片')
    rawURL = URL.createObjectURL(file)
    const img = new Image(); img.src = rawURL; await img.decode()
    const w = img.naturalWidth, h = img.naturalHeight
    if (w > 4096 || h > 4096 || w * h > 12000000) throw new Error('图片尺寸过大，请选择边长不超过 4096、总像素不超过 1200 万的图片')
    const canvas = document.createElement('canvas'); canvas.width = canvas.height = 256
    const ctx = canvas.getContext('2d'); if (!ctx) throw new Error('浏览器无法处理图片')
    ctx.fillStyle = '#fff'; ctx.fillRect(0, 0, 256, 256)
    const side = Math.min(w, h); ctx.drawImage(img, (w - side) / 2, (h - side) / 2, side, side, 0, 0, 256, 256)
    const blob = await new Promise<Blob>((resolve, reject) => canvas.toBlob(b => b ? resolve(b) : reject(new Error('图片处理失败')), 'image/jpeg', .9))
    if (disposed) return
    clearPreview(); selected.value = blob; preview.value = URL.createObjectURL(blob)
  } catch (e) { error.value = e instanceof Error ? e.message : '图片无法读取' }
  finally { if (rawURL) URL.revokeObjectURL(rawURL); selecting.value = false }
}
async function saveAvatar(reset = false) {
  if (busy.value || (!reset && !selected.value)) return
  busy.value = true; error.value = ''; success.value = ''
  try {
    const result = reset ? await api.resetAvatar() : await api.uploadAvatar(selected.value!)
    emit('updated', result.user); clearPreview(); success.value = reset ? '已恢复默认头像' : '头像已更新'
  } catch (e) { error.value = e instanceof Error ? e.message : '头像保存失败' }
  finally { busy.value = false }
}
async function saveProfile() {
  if (busy.value) return
  busy.value = true; error.value = ''; success.value = ''
  try {
    const { user } = await api.updateProfile(draft); emit('updated', user)
    Object.assign(draft, { name: user.name, bio: user.bio || '', location: user.location || '', website: user.website || '' })
    original.value = JSON.stringify(draft); success.value = '个人资料已保存'
  } catch (e) { error.value = e instanceof Error ? e.message : '保存失败' }
  finally { busy.value = false }
}
</script>
<template>
  <main class="profile-page" @keydown.esc="close">
    <div class="profile-shell">
      <button ref="back" class="text-btn profile-back" :disabled="busy || selecting" @click="close">← 返回知识库</button>
      <header class="profile-heading"><p>MathWiki LAB / 账户</p><h1>个人主页</h1><span>留下一点关于你的介绍。</span></header>
      <div class="profile-grid">
        <section class="profile-card profile-identity" aria-label="头像设置">
          <img v-if="preview" class="avatar-preview" :src="preview" alt="新头像预览，尚未保存" />
          <UserAvatar v-else :src="user.avatar" :name="user.name" :size="88" />
          <h2>{{ user.name }}</h2><p class="profile-handle">@{{ user.username }}</p><span class="profile-role">{{ user.role === 'admin' ? '管理员' : '普通用户' }}</span>
          <label class="avatar-picker" :class="{ disabled: busy || selecting || !configured }">{{ selecting ? '处理图片中…' : '选择新头像' }}<input type="file" accept="image/jpeg,image/png" :disabled="busy || selecting || !configured" @change="choose" /></label>
          <template v-if="selected"><button class="primary-btn" :disabled="busy || selecting" @click="saveAvatar()">{{ busy ? '保存中…' : '保存头像' }}</button><button class="text-btn" :disabled="busy" @click="clearPreview">取消选择</button></template>
          <button v-else-if="user.avatar" class="text-btn" :disabled="busy || selecting" @click="saveAvatar(true)">恢复默认头像</button>
          <small>JPG / PNG · 最大 5 MB<br>居中裁剪，保存为 256 × 256</small>
          <p v-if="configured === false" class="profile-storage-note">头像存储尚未配置。管理员填写后端 COS 配置后即可使用。</p>
        </section>
        <form class="profile-card profile-form" @submit.prevent="saveProfile">
          <h2>个人资料</h2><p class="profile-form-note">昵称与头像会显示在评论和批注中。</p>
          <fieldset :disabled="busy"><label>昵称<input v-model="draft.name" name="nickname" autocomplete="nickname" maxlength="40" required placeholder="希望大家如何称呼你" /></label>
          <label>个人简介<textarea v-model="draft.bio" name="bio" rows="4" maxlength="400" placeholder="正在学习什么？对哪些领域感兴趣？" /><small>{{ Array.from(draft.bio).length }} / 400</small></label>
          <label>所在地 <span>选填</span><input v-model="draft.location" name="location" autocomplete="address-level2" maxlength="80" placeholder="城市或地区" /></label>
          <label>个人网站 <span>选填</span><input v-model="draft.website" name="website" type="url" maxlength="512" placeholder="https://" /></label></fieldset>
          <div class="profile-save"><span>账号与角色不可自行修改</span><button class="primary-btn" :disabled="busy || !dirty || !draft.name.trim()">{{ busy ? '保存中…' : '保存资料' }}</button></div>
        </form>
      </div>
      <p v-if="error" class="profile-message inline-error" role="alert">{{ error }}</p>
      <p v-if="success" class="profile-message" role="status">{{ success }}</p>
    </div>
  </main>
</template>
<style scoped>
.profile-page { position: fixed; inset: 0; z-index: 145; overflow-y: auto; background: var(--sidebar); color: var(--text); padding: 28px 24px 64px; }
.profile-shell { max-width: 820px; margin: 0 auto; }
.profile-back { margin-left: -8px; }
.profile-heading { margin: 40px 0 28px; }
.profile-heading p { color: var(--muted); letter-spacing: 1px; font-size: 11px; }
.profile-heading h1 { font-size: 28px; font-weight: 600; margin: 12px 0; }
.profile-heading > span { font-size: 13px; color: var(--muted); }
.profile-grid { display: grid; grid-template-columns: 240px minmax(0, 1fr); gap: 20px; align-items: start; }
.profile-card { background: var(--paper); border: 1px solid var(--line); border-radius: 10px; padding: 28px; }
.profile-identity { display: flex; align-items: center; flex-direction: column; text-align: center; gap: 12px; }
.profile-card h2 { font-size: 16px; font-weight: 600; margin: 0; overflow-wrap: anywhere; }
.profile-identity h2 { margin-top: 6px; }
.profile-handle { font-size: 12px; color: var(--muted); margin: -5px 0 0; overflow-wrap: anywhere; max-width: 100%; }
.profile-role { font-size: 10px; padding: 4px 9px; border-radius: 4px; background: var(--sidebar); color: var(--muted); }
.avatar-preview { width: 88px; height: 88px; object-fit: cover; border-radius: 50%; }
.avatar-picker { position: relative; padding: 9px 16px; border: 1px solid var(--line); border-radius: 5px; cursor: pointer; font-size: 12px; margin-top: 12px; }
.avatar-picker input { position: absolute; inset: 0; opacity: 0; width: 100%; cursor: pointer; }
.avatar-picker:focus-within { outline: 2px solid var(--red); outline-offset: 2px; }
.avatar-picker.disabled { opacity: .5; cursor: not-allowed; }
.profile-identity small, .profile-storage-note { font-size: 11px; color: var(--muted); line-height: 1.9; }
.profile-form-note { font-size: 12px; color: var(--muted); line-height: 1.8; margin-bottom: 24px; }
.profile-form fieldset { border: 0; padding: 0; margin: 0; min-width: 0; }
.profile-form label { display: block; font-size: 12px; margin-bottom: 20px; }
.profile-form label > span { color: var(--muted); font-size: 10px; margin-left: 4px; }
.profile-form input, .profile-form textarea { display: block; width: 100%; margin-top: 9px; padding: 11px 12px; border: 1px solid var(--line); border-radius: 5px; background: var(--sidebar); color: inherit; font: inherit; line-height: 1.7; }
.profile-form textarea { resize: vertical; min-height: 100px; }
.profile-form input:focus, .profile-form textarea:focus { outline: 2px solid var(--red); outline-offset: 2px; }
.profile-form label small { display: block; text-align: right; margin-top: 6px; color: var(--muted); font-size: 10px; }
.profile-save { display: flex; gap: 12px; align-items: center; justify-content: space-between; padding-top: 18px; border-top: 1px solid var(--line); }
.profile-save > span { font-size: 10px; color: var(--muted); }
.profile-message { padding: 14px; font-size: 12px; line-height: 1.7; border: 1px solid var(--line); background: var(--paper); border-radius: 6px; }
@media (max-width: 620px) { .profile-grid { grid-template-columns: 1fr; } .profile-heading { margin-top: 24px; } .profile-card { padding: 24px; } .profile-page { padding: 20px 16px 40px; } }
</style>
