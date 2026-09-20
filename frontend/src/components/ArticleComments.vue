<script setup lang="ts">
import { ref, watch } from 'vue'
import { api } from '../api'
import UserAvatar from './UserAvatar.vue'
import type { User, Comment } from '../types'
const props = defineProps<{ articleId: number; user: User | null; online: boolean }>()
const emit = defineEmits<{ login: [] }>()
const items = ref<Comment[]>([]), total = ref(0), page = ref(1)
const draft = ref(''), error = ref(''), loading = ref(false), sending = ref(false)
let generation = 0
async function load(more = false) {
  if (!props.online) return
  const version = ++generation, id = props.articleId, target = more ? page.value + 1 : 1
  loading.value = true; error.value = ''
  try {
    const result = await api.articleComments(id, target)
    if (version !== generation) return
    items.value = more ? [...items.value, ...result.items.filter(c => !items.value.some(old => old.id === c.id))] : result.items
    total.value = result.total; page.value = target
  } catch (e) { if (version === generation) error.value = e instanceof Error ? e.message : '加载失败' }
  finally { if (version === generation) loading.value = false }
}
watch(() => [props.articleId, props.user?.id, props.online], () => { generation++; items.value = []; total.value = 0; draft.value = ''; void load() }, { immediate: true })
async function post() {
  if (!props.user) { emit('login'); return }
  if (sending.value || !draft.value.trim()) return
  const id = props.articleId; sending.value = true; error.value = ''
  try { await api.postArticleComment(id, draft.value); if (id === props.articleId) { draft.value = ''; await load() } }
  catch (e) { if (id === props.articleId) error.value = e instanceof Error ? e.message : '评论失败' }
  finally { sending.value = false }
}
async function remove(item: Comment) {
  if (!window.confirm('确定删除这条评论？')) return
  const id = props.articleId
  try { await api.deleteArticleComment(item.id); if (id === props.articleId) await load() }
  catch (e) { error.value = e instanceof Error ? e.message : '删除失败' }
}
</script>
<template>
  <section class="article-comments" aria-label="文章讨论">
    <div class="community-heading"><h2>文章讨论 <small>{{ total }}</small></h2><span>交流理解，也欢迎提出疑问</span></div>
    <p v-if="!online" class="service-notice">后端未连接，暂时无法查看评论。</p>
    <form v-else-if="user" @submit.prevent="post">
      <label class="sr-only" for="article-comment">写下你的评论</label>
      <textarea id="article-comment" v-model="draft" maxlength="2000" rows="3" placeholder="分享你的理解，或提出一个具体的问题…" />
      <div class="community-form-footer"><small>{{ draft.length }} / 2000</small><button class="primary-btn" :disabled="sending || !draft.trim()">{{ sending ? '发布中…' : '发布评论' }}</button></div>
    </form>
    <p v-else class="service-notice"><button class="text-btn" @click="emit('login')">登录</button> 后参与文章讨论。</p>
    <p v-if="error" class="inline-error" role="alert">{{ error }} <button @click="load()">重试加载</button></p>
    <div v-for="item in items" :key="item.id" class="article-comment">
      <div class="community-heading"><UserAvatar :src="item.userId === user?.id ? user.avatar : item.avatar" :name="item.userId === user?.id ? user.name : item.author" /><strong>{{ item.userId === user?.id ? user.name : item.author }}</strong><small>{{ new Date(item.createdAt).toLocaleString('zh-CN') }}</small><button v-if="user && (user.role === 'admin' || item.userId === user.id)" class="text-btn danger" @click="remove(item)">删除</button></div>
      <p>{{ item.body }}</p>
    </div>
    <p v-if="online && !loading && !error && !items.length" class="muted">还没有评论，写下第一份思考吧。</p>
    <p v-if="loading" class="muted">正在加载评论…</p>
    <button v-if="items.length < total" class="load-more" :disabled="loading" @click="load(true)">加载更多评论</button>
  </section>
</template>
