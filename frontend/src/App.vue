<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { api } from './api'
import { conversationHistory, handleComposerKey, type ConversationTurn } from './explanationStream'
import WikiContent from './components/WikiContent.vue'
import AuthPage from './components/AuthPage.vue'
import ArticleComments from './components/ArticleComments.vue'
import ModerationPanel from './components/ModerationPanel.vue'
import MyFeedbackPanel from './components/MyFeedbackPanel.vue'
import { articleReviewLabel, type ModerationSection } from './moderation'
import { searchRank, searchSortParam } from './searchRanking'
import ProfilePage from './components/ProfilePage.vue'
import UserAvatar from './components/UserAvatar.vue'
import type { User } from './types'
import { articles as mockArticles, seedAnnotations } from './mock'
import type { Annotation, Article, Stats, Category, Comment, Capabilities } from './types'

type PanelTab = 'ai' | 'notes' | 'info'
type Toast = { text: string; action?: string }

const items = ref<Article[]>([...mockArticles])
const activeArticle = ref<Article>(items.value[0])
const query = ref('')
const activeCollection = ref('全部条目')
const panelTab = ref<PanelTab>('ai')
const rightOpen = ref(true)
const sidebarOpen = ref(false)
const level = ref('本科生')
const explainStyle = ref('直觉 + 严谨')
const explainPrompt = ref('为什么边权不能是负数？')
const explaining = ref(false)
const verificationStage = ref(0)
const conversation = ref<ConversationTurn[]>([])
let explainController: AbortController | null = null

function clearConversation() {
  explainController?.abort()
  explainController = null
  explaining.value = false
  conversation.value = []
}

function stopExplanation() { explainController?.abort() }

function followConversation(force = false) {
  const panel = document.querySelector<HTMLElement>('.ai-panel')
  if (!panel || (!force && panel.scrollHeight - panel.scrollTop - panel.clientHeight > 180)) return
  nextTick(() => { if (panel.isConnected) panel.scrollTop = panel.scrollHeight })
}
const annotations = ref<Annotation[]>([...seedAnnotations])
const selectedQuote = ref('')
const selectionPos = ref({ x: 0, y: 0 })
const composerOpen = ref(false)
const annotationText = ref('')
const toast = ref<Toast | null>(null)
const searchInput = ref<HTMLInputElement | null>(null)
const listening = ref(false)
const sortMode = ref('最近更新')
const searchSortMode = ref('相关度')
const displayedSortMode = computed({
  get: () => query.value.trim() ? searchSortMode.value : sortMode.value,
  set: (value: string) => { if (query.value.trim()) searchSortMode.value = value; else sortMode.value = value }
})
const activeType = ref('全部')
const focusMode = ref(false)
const apiState = ref<'loading' | 'online' | 'offline'>('loading')
const stats = ref<Stats | null>(null)
const loadingArticle = ref(false)
const storedTheme = localStorage.getItem('mathwiki-theme')
const darkMode = ref(storedTheme ? storedTheme === 'dark' : true)
const subjects = ref<Category[]>([])
const collapsedSubjects = ref<string[]>([])
const capabilities = ref<Capabilities>({ aiConfigured: false, speechConfigured: false })
const loadingList = ref(false)
const listError = ref('')
const detailError = ref('')
const totalArticles = ref(0)
const page = ref(1)
const menu = ref<'library' | 'article' | null>(null)
const dialog = ref<'article' | 'category' | 'feedback' | 'connection' | null>(null)
const dialogError = ref('')
const submitting = ref(false)
const categoryName = ref('')
const feedbackText = ref('')
const feedbackArticleId = ref(0)
const articleDraft = reactive({ title: '', category: '数学基础', kind: '概念', difficulty: '入门', content: '' })
const noteSort = ref('最相关')
const noteFilter = ref('')
const expandedNotes = ref<number[]>([])
const commentLists = ref<Record<number, Comment[]>>({})
const replyDrafts = ref<Record<number, string>>({})
const replyErrors = ref<Record<number, string>>({})
const replyBusy = ref<number[]>([])
const favoriteBusy = ref(false)
const annotationBusy = ref(false)
const likesBusy = ref<number[]>([])
const user = ref<User | null>(null)
const authOpen = ref(false)
const moderationOpen = ref(false)
const feedbackOpen = ref(false)
const articleDeleteBusy = ref(false)
const moderationOptions = reactive({ initialSection: 'articles' as ModerationSection, initialStatus: 'pending', initialQuery: '' })
function openModeration(article?: Article) {
  if (user.value?.role !== 'admin') return
  moderationOptions.initialSection = 'articles'
  moderationOptions.initialStatus = !article || article.reviewStatus === 'community_pending' ? 'pending' : article.reviewStatus === 'approved' ? 'approved' : article.reviewStatus === 'rejected' ? 'rejected' : 'unreviewed'
  moderationOptions.initialQuery = article?.title || ''
  menu.value = null; moderationOpen.value = true
}
function openMyFeedback() { if (requireAccount()) feedbackOpen.value = true }
async function moderationChanged(section: ModerationSection) {
  if (section === 'annotations') { await refreshNotes(); return }
  if (section !== 'articles') return
  try { await loadArticles(); await refreshStats(); subjects.value = await api.getCategories(); if (readerVisible.value) await selectArticle(activeArticle.value) }
  catch { showToast('操作已保存，知识库刷新失败，请稍后刷新页面') }
}
async function removeActiveArticle() {
  if (articleDeleteBusy.value || user.value?.role !== 'admin' || !requireOnline()) return
  const article = activeArticle.value
  if (!window.confirm(`将「${article.title}」移入回收站？正文和关联讨论将隐藏，可在审核管理中恢复。`)) return
  articleDeleteBusy.value = true; menu.value = null
  try { await api.deleteArticle(article.id); annotations.value = []; commentLists.value = {}; clearConversation(); await moderationChanged('articles'); showToast('条目已移入回收站，可在审核管理中恢复') }
  catch (e) { showToast(e instanceof Error ? e.message : '删除失败') }
  finally { articleDeleteBusy.value = false }
}
const profileOpen = ref(false)
function profileUpdated(nextUser: User) {
  user.value = nextUser
  for (const note of annotations.value) if (note.userId === nextUser.id) { note.author = nextUser.name; note.avatar = nextUser.avatar }
  for (const list of Object.values(commentLists.value)) for (const comment of list) if (comment.userId === nextUser.id) { comment.author = nextUser.name; comment.avatar = nextUser.avatar }
}
const accountBusy = ref(false)
function requestLogin() { authOpen.value = true }
function sessionExpired() {
  clearConversation()
  feedbackOpen.value = false
  profileOpen.value = false
  user.value = null; stats.value = null; annotations.value = []; commentLists.value = {}; replyDrafts.value = {}
  items.value = items.value.map(item => ({ ...item, saved: false })); activeArticle.value.saved = false
  activeCollection.value = '全部条目'; moderationOpen.value = false; requestLogin()
}
function requireAccount() { if (user.value) return true; requestLogin(); return false }
function canDelete(owner?: number) { return !!user.value && (user.value.role === 'admin' || user.value.id === owner) }
async function accountChanged(nextUser: User | null) {
  clearConversation()
  feedbackOpen.value = false
  profileOpen.value = false
  user.value = nextUser; authOpen.value = false; moderationOpen.value = false
  ++articleRequest; ++listRequest
  annotations.value = []; commentLists.value = {}; replyDrafts.value = {}; annotationText.value = ''
  activeCollection.value = '全部条目'
  await loadBackendContent()
}
async function logout() {
  if (accountBusy.value) return
  accountBusy.value = true
  try { await api.logout(); await accountChanged(null); showToast('已退出登录') }
  catch (e) { showToast(e instanceof Error ? e.message : '退出失败') }
  finally { accountBusy.value = false }
}
async function refreshNotes() {
  const id = activeArticle.value.id
  try { const notes = await api.getAnnotations(id); if (id === activeArticle.value.id) annotations.value = notes; await refreshStats() }
  catch (e) { showToast(e instanceof Error ? e.message : '批注刷新失败') }
}
async function deleteNote(note: Annotation) {
  if (!window.confirm('确定删除这条批注及其回复？')) return
  try { await api.deleteAnnotation(note.id); await refreshNotes() }
  catch (e) { showToast(e instanceof Error ? e.message : '删除失败') }
}
async function deleteReply(note: Annotation, comment: Comment) {
  if (!window.confirm('确定删除这条回复？')) return
  try { await api.deleteComment(comment.id); commentLists.value[note.id] = (commentLists.value[note.id] ?? []).filter(c => c.id !== comment.id); note.replies = Math.max(0, note.replies - 1) }
  catch (e) { showToast(e instanceof Error ? e.message : '删除失败') }
}
let mediaRecorder: MediaRecorder | null = null
let audioStream: MediaStream | null = null
let audioChunks: Blob[] = []

const collections = computed(() => [
  { name: '全部条目', icon: 'library', count: stats.value?.articleCount ?? items.value.length },
  { name: '我的收藏', icon: 'star', count: stats.value?.favoriteCount ?? items.value.filter(item => item.saved).length },
  { name: '最近阅读', icon: 'clock', count: stats.value?.recentCount ?? 0 },
  { name: '我的批注', icon: 'note', count: stats.value?.annotationCount ?? annotations.value.length }
])

const filteredArticles = computed(() => {
  if (apiState.value === 'online') return items.value
  const q = query.value.trim().toLowerCase()
  let result = items.value.filter(a => {
    if (activeCollection.value === '我的收藏' && !a.saved) return false
    if (activeCollection.value === '最近阅读') return false
    if (activeCollection.value === '我的批注' && !seedAnnotations.some(n => n.articleId === a.id)) return false
    if (!['全部条目', '我的收藏', '最近阅读', '我的批注'].includes(activeCollection.value) && ![a.title, a.category, ...a.tags].join(' ').includes(activeCollection.value)) return false
    if (activeType.value !== '全部' && a.kind !== activeType.value) return false
    return !q || Number.isFinite(searchRank(a, q))
  })
  if (q && displayedSortMode.value === '相关度') result = [...result].sort((a, b) => searchRank(a, q) - searchRank(b, q))
  if (displayedSortMode.value === '标题') result = [...result].sort((a, b) => a.title.localeCompare(b.title, 'zh-CN'))
  if (displayedSortMode.value === '难度') result = [...result].sort((a, b) => ['入门', '进阶', '高级'].indexOf(a.difficulty) - ['入门', '进阶', '高级'].indexOf(b.difficulty))
  return result
})

const readerVisible = computed(() => filteredArticles.value.some(article => article.id === activeArticle.value.id))
const articleAnnotations = computed(() => annotations.value.filter(a => a.articleId === activeArticle.value.id))
const visibleNotes = computed(() => {
  const notes = articleAnnotations.value.filter(note => !noteFilter.value || noteFilter.value.includes(note.quote))
  return [...notes].sort((a, b) => noteSort.value === '最新' ? new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime() : b.likes - a.likes)
})

function icon(name: string, size = 18) {
  const paths: Record<string, string> = {
    library: '<path d="M4 4.5h12v13H4z"/><path d="M7 2.5h10v13"/>',
    star: '<path d="m10 2 2.3 4.7 5.2.8-3.8 3.7.9 5.3-4.6-2.5-4.6 2.5.9-5.3-3.8-3.7 5.2-.8z"/>',
    clock: '<circle cx="10" cy="10" r="7.5"/><path d="M10 5.8v4.6l3.1 1.8"/>',
    note: '<path d="M4 3.5h12v13H4z"/><path d="M7 7h6M7 10h6M7 13h3"/>',
    search: '<circle cx="8.5" cy="8.5" r="5.5"/><path d="m13 13 4 4"/>',
    plus: '<path d="M10 4v12M4 10h12"/>',
    more: '<circle cx="4" cy="10" r=".8" fill="currentColor"/><circle cx="10" cy="10" r=".8" fill="currentColor"/><circle cx="16" cy="10" r=".8" fill="currentColor"/>',
    chevron: '<path d="m7 4 6 6-6 6"/>',
    bookmark: '<path d="M5 3.5h10v14l-5-3-5 3z"/>',
    panel: '<path d="M3 4h14v12H3zM12 4v12"/>',
    spark: '<path d="m10 2 1.2 4.1L15 8l-3.8 1.9L10 14l-1.2-4.1L5 8l3.8-1.9z"/><path d="m16 13 .6 2.1 1.9.9-1.9.9L16 19l-.6-2.1-1.9-.9 1.9-.9z"/>',
    comment: '<path d="M3 4h14v10H8l-4 3v-3H3z"/>',
    info: '<circle cx="10" cy="10" r="7.5"/><path d="M10 9v5M10 6.2v.2"/>',
    mic: '<rect x="7" y="3" width="6" height="10" rx="3"/><path d="M4.5 10.5a5.5 5.5 0 0 0 11 0M10 16v2"/>',
    send: '<path d="m3 4 14 6-14 6 2-6zM5 10h6"/>',
    close: '<path d="m5 5 10 10M15 5 5 15"/>',
    check: '<path d="m4 10 4 4 8-8"/>',
    quote: '<path d="M4 5h5v5H6v4H4V5zM11 5h5v5h-3v4h-2V5z"/>',
    heart: '<path d="M10 17S3 13 3 7.8A3.8 3.8 0 0 1 10 6a3.8 3.8 0 0 1 7 1.8C17 13 10 17 10 17z"/>',
    external: '<path d="M11 4h5v5M9 11l7-7M16 11v5H4V4h5"/>',
    moon: '<path d="M16.6 12.4A7.2 7.2 0 0 1 7.6 3.4a7.2 7.2 0 1 0 9 9z"/>',
    sun: '<circle cx="10" cy="10" r="3.3"/><path d="M10 1.8v2M10 16.2v2M1.8 10h2M16.2 10h2M4.2 4.2l1.4 1.4M14.4 14.4l1.4 1.4M15.8 4.2l-1.4 1.4M5.6 14.4l-1.4 1.4"/>',
    focus: '<path d="M3 7V3h4M13 3h4v4M17 13v4h-4M7 17H3v-4"/>'
  }
  return `<svg viewBox="0 0 20 20" width="${size}" height="${size}" aria-hidden="true">${paths[name] || paths.info}</svg>`
}

let articleRequest = 0

async function selectArticle(article: Article) {
  clearConversation()
  const requestId = ++articleRequest
  activeArticle.value = article
  detailError.value = ''
  annotations.value = []
  noteFilter.value = ''
  expandedNotes.value = []
  explainPrompt.value = `请解释${article.title}的核心思想`
  selectedQuote.value = ''
  composerOpen.value = false
  sidebarOpen.value = false
  document.querySelector('.reader-scroll')?.scrollTo({ top: 0, behavior: 'smooth' })
  if (apiState.value !== 'online') {
    annotations.value = seedAnnotations.filter(note => note.articleId === article.id)
    return
  }
  loadingArticle.value = true
  try {
    const [detail, notes] = await Promise.all([api.getArticle(article.slug), api.getAnnotations(article.id)])
    if (requestId !== articleRequest) return
    activeArticle.value = detail
    annotations.value = notes
    items.value = items.value.map(item => item.id === detail.id ? { ...item, ...detail } : item)
    if (user.value) await api.markRead(detail.id)
    await refreshStats()
  } catch {
    if (requestId === articleRequest) detailError.value = '条目或批注加载失败，请点击重试'
  } finally {
    if (requestId === articleRequest) loadingArticle.value = false
  }
}

function chooseCollection(name: string) {
  if (['我的收藏', '最近阅读', '我的批注'].includes(name) && !requireAccount()) return
  activeCollection.value = name
  query.value = ''
  activeType.value = '全部'
  menu.value = null
  if (name === '我的批注') { panelTab.value = 'notes'; rightOpen.value = true }
  sidebarOpen.value = false
}

async function refreshStats() {
  stats.value = await api.getStats()
}

let listRequest = 0
async function loadArticles(append = false) {
  if (apiState.value !== 'online') return
  const requestId = ++listRequest
  loadingList.value = true
  listError.value = ''
  const collectionKeys: Record<string, string> = { '全部条目': '', '我的收藏': 'favorites', '最近阅读': 'recent', '我的批注': 'annotated' }
  const params = {
    page: String(append ? page.value + 1 : 1), q: query.value.trim(),
    collection: collectionKeys[activeCollection.value] ?? '',
    category: activeCollection.value in collectionKeys ? '' : activeCollection.value,
    kind: activeType.value === '全部' ? '' : activeType.value,
    sort: searchSortParam(displayedSortMode.value)
  }
  try {
    const result = await api.listArticles(params)
    if (requestId !== listRequest) return
    page.value = Number(params.page)
    items.value = append ? [...items.value, ...result.items] : result.items
    totalArticles.value = result.total
    if (!append && result.items.length && !result.items.some(item => item.id === activeArticle.value.id)) await selectArticle(result.items[0])
  } catch (error) {
    if (requestId === listRequest) listError.value = error instanceof Error ? error.message : '条目加载失败'
  } finally {
    if (requestId === listRequest) loadingList.value = false
  }
}

async function loadBackendContent() {
  apiState.value = 'loading'
  try {
    const [counts, categories, services] = await Promise.all([api.getStats(), api.getCategories(), api.getCapabilities()])
    stats.value = counts
    subjects.value = categories
    capabilities.value = services
    items.value = []
    apiState.value = 'online'
    await loadArticles()
    if (items.value.length) await selectArticle(items.value[0])
  } catch {
    apiState.value = 'offline'
    items.value = [...mockArticles]
    activeArticle.value = items.value[0]
    annotations.value = [...seedAnnotations]
    stats.value = null
    subjects.value = [...new Set(mockArticles.map(article => article.category))].map(name => ({ name, count: mockArticles.filter(article => article.category === name).length, children: [] }))
  }
}

let searchTimer: number | undefined
watch(query, (value, previous) => {
  if (value.trim() && !previous.trim()) searchSortMode.value = '相关度'
})
watch([query, activeCollection, activeType, sortMode, searchSortMode], () => {
  window.clearTimeout(searchTimer)
  ++listRequest
  searchTimer = window.setTimeout(() => void loadArticles(), 220)
})

function handleSelection() {
  const selection = window.getSelection()
  const text = selection?.toString().trim() || ''
  if (!text || text.length < 2 || text.length > 240) {
    if (!composerOpen.value) selectedQuote.value = ''
    return
  }
  const range = selection!.getRangeAt(0)
  if (!(range.commonAncestorContainer.parentElement?.closest('.article-body'))) return
  const rect = range.getBoundingClientRect()
  selectedQuote.value = text
  selectionPos.value = { x: Math.min(window.innerWidth - 190, Math.max(12, rect.left + rect.width / 2 - 88)), y: Math.max(62, rect.top - 48) }
}

function openAnnotation() {
  if (!requireAccount()) return
  composerOpen.value = true
  panelTab.value = 'notes'
  rightOpen.value = true
  nextTick(() => document.querySelector<HTMLTextAreaElement>('.annotation-input')?.focus())
}

function explainSelection() {
  explainPrompt.value = `请解释这句话：${selectedQuote.value}`
  panelTab.value = 'ai'
  rightOpen.value = true
  runExplain()
}

async function runExplain() {
  if (!requireAccount()) return
  if (explaining.value || !explainPrompt.value.trim()) return
  if (!requireOnline()) return
  const requestedArticle = activeArticle.value.id
  const question = explainPrompt.value.trim()
  const history = conversationHistory(conversation.value)
  const controller = new AbortController()
  explainController = controller
  const index = conversation.value.length
  conversation.value.push({ question, text: '' })
  const turn = conversation.value[index]!
  explainPrompt.value = ''
  followConversation(true)
  explaining.value = true
  verificationStage.value = 1
  try {
    await api.streamExplanation({ articleId: requestedArticle, question, audience: level.value, style: explainStyle.value, history }, event => {
      if (explainController !== controller || activeArticle.value.id !== requestedArticle) return
      if (event.type === 'delta') turn.text += event.text || ''
      if (event.type === 'phase') verificationStage.value = event.text === 'verifying' ? 3 : 2
      if (event.type === 'done') turn.result = event.explanation
      followConversation()
    }, controller.signal)
  } catch (error) {
    if (explainController === controller) {
      turn.error = controller.signal.aborted ? '已停止。以上内容尚未完成核验。' : error instanceof Error ? error.message : '解释生成失败，请重试'
      if (!explainPrompt.value) explainPrompt.value = question
    }
  } finally {
    if (explainController === controller) { verificationStage.value = 4; explaining.value = false; explainController = null }
  }
}

async function saveAnnotation() {
  if (!requireAccount()) return
  if (annotationBusy.value || !annotationText.value.trim() || !selectedQuote.value || !requireOnline()) return
  annotationBusy.value = true
  const articleId = activeArticle.value.id
  const body = annotationText.value.trim()
  const quote = selectedQuote.value
  try {
    const note = await api.createAnnotation({ articleId, quote, body, color: '#e5a84b' })
    if (activeArticle.value.id === articleId) annotations.value.unshift(note)
    await refreshStats()
  } catch (error) {
    showToast(error instanceof Error ? error.message : '批注发布失败')
    return
  } finally {
    annotationBusy.value = false
  }
  annotationText.value = ''
  composerOpen.value = false
  selectedQuote.value = ''
  showToast(user.value?.role === 'admin' ? '批注已公开发布' : '批注已提交，审核通过后公开')
}

async function toggleSaved(article: Article) {
  if (!requireAccount()) return
  if (favoriteBusy.value || !requireOnline()) return
  favoriteBusy.value = true
  const previous = article.saved
  try {
    const result = await api.setFavorite(article.id, !previous)
    article.saved = result.saved
    items.value = items.value.map(item => item.id === article.id ? { ...item, saved: result.saved } : item)
    if (activeArticle.value.id === article.id) activeArticle.value.saved = result.saved
    await refreshStats()
    if (activeCollection.value === '我的收藏') await loadArticles()
    showToast(result.saved ? '已加入我的收藏' : '已从收藏中移除')
  } catch {
    article.saved = previous
    showToast('收藏状态保存失败')
  } finally {
    favoriteBusy.value = false
  }
}

async function likeNote(note: Annotation) {
  if (!requireAccount()) return
  if (likesBusy.value.includes(note.id) || !requireOnline()) return
  likesBusy.value.push(note.id)
  const previous = note.likes
  note.likes = Math.max(0, note.likes + (note.liked ? -1 : 1))
  try {
    const result = await api.likeAnnotation(note.id, !note.liked)
    note.likes = result.likes
    note.liked = result.liked
  } catch {
    note.likes = previous
    showToast('点赞失败，请稍后重试')
  } finally {
    likesBusy.value = likesBusy.value.filter(id => id !== note.id)
  }
}

async function sendFeedback(kind: string) {
  if (!requireOnline()) return
  try {
    await api.feedback(activeArticle.value.id, kind)
    showToast(kind === 'helpful' ? '感谢你的反馈' : '已记录，将用于改进解释')
  } catch {
    showToast('反馈提交失败')
  }
}

function formatDate(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const diff = Date.now() - date.getTime()
  if (diff >= 0 && diff < 60_000) return '刚刚'
  if (diff >= 0 && diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`
  if (diff >= 0 && diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`
  return new Intl.DateTimeFormat('zh-CN', { month: 'short', day: 'numeric' }).format(date)
}

function requireOnline() {
  if (apiState.value === 'online') return true
  showToast('后端未连接，当前样例仅供浏览。请在知识库菜单中重新连接')
  return false
}

function toggleSubject(name: string) {
  collapsedSubjects.value = collapsedSubjects.value.includes(name) ? collapsedSubjects.value.filter(item => item !== name) : [...collapsedSubjects.value, name]
}

function openDialog(name: 'article' | 'category' | 'feedback' | 'connection') {
  if (name !== 'connection' && !requireAccount()) return
  if ((name === 'article' || name === 'category') && user.value?.role !== 'admin') { showToast('创建条目和分类需要管理员权限'); return }
  dialog.value = name
  dialogError.value = ''
  menu.value = null
  if (name === 'feedback') { feedbackArticleId.value = activeArticle.value.id; feedbackText.value = '' }
  nextTick(() => document.querySelector<HTMLElement>('.action-dialog input, .action-dialog textarea, .action-dialog button')?.focus())
}

async function submitDialog() {
  if (submitting.value || !requireOnline()) return
  submitting.value = true
  dialogError.value = ''
  try {
    if (dialog.value === 'category') {
      await api.createCategory(categoryName.value.trim())
      subjects.value = await api.getCategories()
      chooseCollection(categoryName.value.trim())
      categoryName.value = ''
      showToast('分类已保存，可以在新建条目时使用')
    } else if (dialog.value === 'article') {
      const article = await api.createArticle(articleDraft)
      activeCollection.value = '全部条目'
      activeType.value = '全部'
      query.value = ''
      sortMode.value = '最近更新'
      window.clearTimeout(searchTimer)
      await nextTick()
      window.clearTimeout(searchTimer)
      await loadArticles()
      openModeration(article)
      subjects.value = await api.getCategories()
      Object.assign(articleDraft, { title: '', content: '' })
      showToast('条目已提交审核，通过后会在知识库公开')
    } else if (dialog.value === 'feedback') {
      if (!feedbackText.value.trim()) throw new Error('请填写补充或纠错内容')
      await api.feedback(feedbackArticleId.value, 'content_suggestion', feedbackText.value.trim())
      showToast('建议已提交，可在左侧「我的反馈」查看处理进度')
    }
    dialog.value = null
  } catch (error) {
    dialogError.value = error instanceof Error ? error.message : '保存失败，请重试'
  } finally { submitting.value = false }
}

async function copyText(text: string) {
  try { await navigator.clipboard.writeText(text); showToast('已复制到剪贴板') }
  catch { showToast('浏览器未允许复制，请选中文字后按 Ctrl+C') }
  menu.value = null
}

function showSectionNotes(content: string) {
  noteFilter.value = content
  panelTab.value = 'notes'
  rightOpen.value = true
}

function annotateSection(content: string) {
  selectedQuote.value = [...content].slice(0, 200).join('')
  openAnnotation()
}

async function openReplies(note: Annotation) {
  if (expandedNotes.value.includes(note.id)) {
    expandedNotes.value = expandedNotes.value.filter(id => id !== note.id)
    return
  }
  expandedNotes.value.push(note.id)
  replyErrors.value[note.id] = ''
  if (!requireOnline()) return
  replyBusy.value.push(note.id)
  try { commentLists.value[note.id] = await api.getComments(note.id) }
  catch (error) { replyErrors.value[note.id] = error instanceof Error ? error.message : '回复加载失败' }
  finally { replyBusy.value = replyBusy.value.filter(id => id !== note.id) }
}

async function submitReply(note: Annotation) {
  if (!requireAccount()) return
  const body = replyDrafts.value[note.id]?.trim()
  if (!body || replyBusy.value.includes(note.id) || !requireOnline()) return
  replyBusy.value.push(note.id)
  replyErrors.value[note.id] = ''
  try {
    const comment = await api.createComment(note.id, body)
    commentLists.value[note.id] = [...(commentLists.value[note.id] ?? []), comment]
    replyDrafts.value[note.id] = ''
    note.replies = commentLists.value[note.id].length
    showToast('回复已保存')
  } catch (error) { replyErrors.value[note.id] = error instanceof Error ? error.message : '回复失败' }
  finally { replyBusy.value = replyBusy.value.filter(id => id !== note.id) }
}

function reportNote(note: Annotation) {
  openDialog('feedback')
  feedbackText.value = `关于批注 #${note.id}（${note.body}）的反馈：`
}

function showToast(text: string) {
  toast.value = { text }
  window.setTimeout(() => toast.value = null, 2400)
}

function syncThemeColor() {
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', darkMode.value ? '#202123' : '#f6f5f2')
}

function toggleTheme() {
  darkMode.value = !darkMode.value
  localStorage.setItem('mathwiki-theme', darkMode.value ? 'dark' : 'light')
  syncThemeColor()
  showToast(darkMode.value ? '已切换到深色模式' : '已切换到浅色模式')
}

async function toggleListening() {
  if (!capabilities.value.speechConfigured) {
    showToast('语音识别尚未接入云服务，请暂时使用文字输入')
    return
  }
  if (listening.value && mediaRecorder?.state === 'recording') {
    mediaRecorder.stop()
    return
  }
  if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined') {
    showToast('当前浏览器不支持录音，请改用文字输入')
    return
  }
  try {
    audioStream = await navigator.mediaDevices.getUserMedia({ audio: true })
    const mimeType = ['audio/webm;codecs=opus', 'audio/ogg;codecs=opus', 'audio/mp4']
      .find(type => MediaRecorder.isTypeSupported(type))
    mediaRecorder = new MediaRecorder(audioStream, mimeType ? { mimeType } : undefined)
    audioChunks = []
    mediaRecorder.ondataavailable = event => {
      if (event.data.size) audioChunks.push(event.data)
    }
    mediaRecorder.onerror = () => {
      listening.value = false
      audioStream?.getTracks().forEach(track => track.stop())
      showToast('录音失败，请检查麦克风权限')
    }
    mediaRecorder.onstop = async () => {
      listening.value = false
      audioStream?.getTracks().forEach(track => track.stop())
      if (!audioChunks.length) {
        showToast('没有检测到可转写的语音')
        return
      }
      try {
        const audio = new Blob(audioChunks, { type: mediaRecorder?.mimeType || 'audio/webm' })
        const result = await api.transcribe(audio)
        explainPrompt.value = result.text
        showToast('语音已转写到输入框')
      } catch (error) {
        showToast(error instanceof Error ? error.message : '语音转写失败')
      }
    }
    mediaRecorder.start()
    listening.value = true
    showToast('正在录音，再次点击结束并转写')
  } catch {
    audioStream?.getTracks().forEach(track => track.stop())
    listening.value = false
    showToast('无法使用麦克风，请检查浏览器权限')
  }
}

function handleShortcut(event: KeyboardEvent) {
  if (moderationOpen.value || feedbackOpen.value || profileOpen.value || authOpen.value) return
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault(); searchInput.value?.focus()
  }
  if (event.key === 'Escape') { selectedQuote.value = ''; composerOpen.value = false; menu.value = null; if (!submitting.value) dialog.value = null }
  if (event.key === 'Tab' && dialog.value) {
    const elements = [...document.querySelectorAll<HTMLElement>('.action-dialog button:not(:disabled), .action-dialog input, .action-dialog select, .action-dialog textarea')]
    const first = elements[0], last = elements[elements.length - 1]
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last?.focus() }
    if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first?.focus() }
  }
}

onMounted(async () => {
  syncThemeColor()
  window.addEventListener('keydown', handleShortcut)
  window.addEventListener('mathwiki:login-required', sessionExpired)
  try { user.value = (await api.me()).user } catch { user.value = null }
  await loadBackendContent()
})
onUnmounted(() => {
  stopExplanation()
  window.removeEventListener('mathwiki:login-required', sessionExpired)
  window.removeEventListener('keydown', handleShortcut)
  window.clearTimeout(searchTimer)
  if (mediaRecorder?.state === 'recording') mediaRecorder.stop()
  audioStream?.getTracks().forEach(track => track.stop())
})
</script>

<template>
  <div class="app-shell" :class="{ 'focus-mode': focusMode, dark: darkMode }" @click="menu = null">
    <AuthPage v-if="authOpen" @close="authOpen = false" @authenticated="accountChanged" />
    <ProfilePage v-if="profileOpen && user" :user="user" @close="profileOpen = false" @updated="profileUpdated" />
    <ModerationPanel v-if="moderationOpen && user?.role === 'admin'" v-bind="moderationOptions" @close="moderationOpen = false" @changed="moderationChanged" />
    <MyFeedbackPanel v-if="feedbackOpen && user" @close="feedbackOpen = false" />
    <header class="topbar" :inert="authOpen || moderationOpen || profileOpen || feedbackOpen">
      <div class="brand-block">
        <button class="icon-btn mobile-only" aria-label="打开知识库" @click="sidebarOpen = !sidebarOpen" v-html="icon('library')" />
        <div class="brand-mark"><span>∑</span></div>
        <div class="brand-copy"><strong>MathWiki</strong><span>LAB</span></div>
      </div>
      <div class="global-search">
        <span v-html="icon('search', 16)" />
        <input ref="searchInput" v-model="query" placeholder="搜索概念、算法或公式" aria-label="搜索知识库" />
        <kbd>Ctrl K</kbd>
      </div>
      <div class="top-actions">
        <button class="icon-btn theme-toggle" :title="darkMode ? '切换浅色模式' : '切换深色模式'" :aria-label="darkMode ? '切换浅色模式' : '切换深色模式'" @click="toggleTheme" v-html="icon(darkMode ? 'sun' : 'moon')" />
        <button class="icon-btn" :class="{ active: focusMode }" title="专注阅读" aria-label="专注阅读" @click="focusMode = !focusMode" v-html="icon('focus')" />
          <button v-if="user?.role === 'admin'" class="icon-btn" title="新建条目" aria-label="新建条目" @click="openDialog('article')" v-html="icon('plus')" />
        <button v-if="user?.role === 'admin'" class="text-btn" @click="openModeration()">审核管理</button>
        <template v-if="user"><button class="account-trigger" title="个人主页" aria-label="打开个人主页" @click="profileOpen = true"><UserAvatar :src="user.avatar" :name="user.name" /><span class="account-name">{{ user.name }}<small>{{ user.role === 'admin' ? '管理员' : '普通用户' }}</small></span></button><button class="text-btn" :disabled="accountBusy" @click="logout">退出</button></template>
        <button v-else class="account-login" @click="requestLogin">登录 / 注册</button>
      </div>
    </header>

    <div class="workspace" :inert="authOpen || moderationOpen || profileOpen || feedbackOpen">
      <aside class="library-pane" :class="{ open: sidebarOpen }">
        <div class="pane-title menu-anchor"><span>知识库</span><button class="quiet-btn" aria-label="知识库菜单" :aria-expanded="menu === 'library'" @click.stop="menu = menu === 'library' ? null : 'library'" v-html="icon('more')" />
          <div v-if="menu === 'library'" class="action-menu" @click.stop><button @click="menu = null; loadBackendContent()">刷新知识库</button><button @click="openDialog('connection')">连接与功能状态</button><button @click="openDialog('category')">新建分类</button></div>
        </div>
        <nav class="collection-list">
          <button v-for="entry in collections" :key="entry.name" :class="{ active: activeCollection === entry.name }" @click="chooseCollection(entry.name)">
            <span v-html="icon(entry.icon, 17)" /><span>{{ entry.name }}</span><em>{{ entry.count }}</em>
          </button>
          <button @click="openMyFeedback"><span v-html="icon('comment', 17)" /><span>我的反馈</span></button>
        </nav>
        <div class="section-label"><span>学科分类</span><button aria-label="新建学科分类" @click="openDialog('category')" v-html="icon('plus', 15)" /></div>
        <div class="subject-tree">
          <div v-for="subject in subjects" :key="subject.name" class="subject-group">
            <div class="subject-row">
              <button v-if="subject.children.length" class="tree-toggle" :aria-label="`${collapsedSubjects.includes(subject.name) ? '展开' : '收起'}${subject.name}`" :aria-expanded="!collapsedSubjects.includes(subject.name)" @click="toggleSubject(subject.name)"><span class="tree-chevron" :class="{ expanded: !collapsedSubjects.includes(subject.name) }" v-html="icon('chevron', 13)" /></button>
              <span v-else class="tree-spacer" />
            <button :class="{ active: activeCollection === subject.name }" @click="chooseCollection(subject.name)">
              <span class="folder-dot" />
              <span>{{ subject.name }}</span><em>{{ subject.count }}</em>
            </button>
            </div>
            <template v-if="!collapsedSubjects.includes(subject.name)"><button v-for="child in subject.children" :key="child.name" class="tree-child" :class="{ active: activeCollection === child.name }" @click="chooseCollection(child.name)">
              <span>{{ child.name }}</span><em>{{ child.count }}</em>
            </button></template>
          </div>
        </div>
        <div class="library-footer">
          <div><span class="sync-dot" :class="{ offline: apiState === 'offline', loading: apiState === 'loading' }" />{{ apiState === 'online' ? (stats?.databaseDriver === 'mysql' ? 'MySQL 已连接' : 'SQLite 开发库已连接') : apiState === 'loading' ? '正在连接知识库' : '演示数据模式' }}</div>
          <small>{{ stats?.articleCount ?? items.length }} 个条目 · {{ stats?.sectionCount ?? activeArticle.sections.length }} 个章节</small>
        </div>
      </aside>
      <div v-if="sidebarOpen" class="mobile-scrim" @click="sidebarOpen = false" />

      <section class="items-pane">
        <div class="items-header">
          <div><h2>{{ activeCollection }}</h2><span>{{ apiState === 'online' ? totalArticles : filteredArticles.length }} 个条目</span></div>
          <select v-model="displayedSortMode" aria-label="排序方式"><option v-if="query.trim()">相关度</option><option>最近更新</option><option>标题</option><option>难度</option></select>
        </div>
        <div class="filter-row">
          <button v-for="type in ['全部', '概念', '算法', '例题']" :key="type" class="filter" :class="{ active: activeType === type }" @click="activeType = type">{{ type }}</button>
        </div>
        <div class="item-list">
          <div v-if="loadingList" class="list-notice" role="status">正在加载…</div>
          <div v-if="listError" class="inline-error" role="alert">{{ listError }}<button @click="loadArticles()">重试</button></div>
          <button v-for="article in filteredArticles" :key="article.id" class="article-card" :class="{ active: activeArticle.id === article.id }" @click="selectArticle(article)">
            <div class="article-card-top"><span class="type-icon">{{ article.kind === '算法' ? 'ƒ' : article.kind === '例题' ? '∑' : '◫' }}</span><span class="difficulty" :data-level="article.difficulty">{{ article.difficulty }}</span><span class="saved" :class="{ on: article.saved }" v-html="icon('bookmark', 15)" /></div>
            <h3>{{ article.title }}</h3><p>{{ article.excerpt }}</p>
            <div class="article-meta"><span>{{ article.category }}</span><span>{{ article.readMinutes }} 分钟</span></div>
          </button>
          <div v-if="!loadingList && !filteredArticles.length && !listError" class="empty-state"><span v-html="icon('search', 28)" /><strong>当前分类暂无条目</strong><small>{{ query ? '试试其他关键词，或回到全部条目' : '可新建条目，或选择其他分类' }}</small><button class="text-btn" @click="chooseCollection('全部条目')">查看全部条目</button></div>
          <button v-if="items.length < totalArticles && apiState === 'online'" class="load-more" :disabled="loadingList" @click="loadArticles(true)">加载更多（已显示 {{ items.length }} / {{ totalArticles }}）</button>
        </div>
      </section>

      <main class="reader-pane">
        <div v-if="readerVisible" class="reader-toolbar">
          <div class="breadcrumb"><span>知识库</span><b>/</b><span>{{ activeArticle.category.split(' · ')[0] }}</span><b>/</b><strong>{{ activeArticle.title }}</strong></div>
          <div class="reader-actions">
            <button class="icon-btn" :disabled="favoriteBusy" :aria-pressed="activeArticle.saved" :class="{ active: activeArticle.saved }" title="收藏" aria-label="收藏当前条目" @click="toggleSaved(activeArticle)" v-html="icon('bookmark')" />
            <div class="menu-anchor"><button class="icon-btn" title="更多" aria-label="条目更多操作" :aria-expanded="menu === 'article'" @click.stop="menu = menu === 'article' ? null : 'article'" v-html="icon('more')" />
              <div v-if="menu === 'article'" class="action-menu" @click.stop><button @click="copyText(activeArticle.title + '\n' + activeArticle.excerpt + '\n\n' + activeArticle.sections.map(s => s.title + '\n' + s.content + (s.formula ? '\n' + s.formula : '')).join('\n\n'))">复制条目正文</button><button @click="openDialog('feedback')">提出补充 / 纠错</button><button @click="panelTab = 'info'; rightOpen = true; menu = null">查看条目信息</button><template v-if="user?.role === 'admin'"><button @click="openModeration(activeArticle)">审核此条目</button><button class="danger" :disabled="articleDeleteBusy" @click="removeActiveArticle">删除条目</button></template></div>
            </div>
            <button class="icon-btn" :class="{ active: rightOpen }" title="右侧面板" aria-label="显示或隐藏右侧面板" @click="rightOpen = !rightOpen" v-html="icon('panel')" />
          </div>
        </div>
        <div v-if="readerVisible" class="reader-scroll" @mouseup="handleSelection">
          <div v-if="loadingArticle" class="content-loading"><i /><span>正在加载条目正文…</span></div>
          <div v-if="detailError" class="inline-error" role="alert">{{ detailError }}<button @click="selectArticle(activeArticle)">重试</button></div>
          <article v-if="!loadingArticle && !detailError" class="article-body">
            <div class="article-kicker">{{ activeArticle.category }}</div>
            <h1>{{ activeArticle.title }}</h1>
            <p v-if="activeArticle.contentVersion !== 2" class="lead">{{ activeArticle.excerpt }}</p>
            <div class="article-stats"><span>最后更新：{{ formatDate(activeArticle.updatedAt) }}</span><span>预计阅读 {{ activeArticle.readMinutes }} 分钟</span><span>{{ activeArticle.license ?? 'CC BY-SA 4.0' }}</span></div>
            <details v-if="activeArticle.contentWarnings?.length" class="content-quality"><summary>内容转换说明 · 部分模板待展开</summary><p v-for="warning in activeArticle.contentWarnings" :key="warning">{{ warning }}</p></details>
            <details v-if="activeArticle.sections.length > 3" class="article-toc"><summary>本页目录 · {{ activeArticle.sections.filter(s => s.position !== 1000001).length }} 节</summary><nav><a v-for="section in activeArticle.sections" :key="section.id" :href="`#section-${section.id}`" :class="{ nested: (section.level ?? 2) > 2 }">{{ section.title }}</a></nav></details>
            <div class="article-rule" />
            <section v-for="(section, index) in activeArticle.sections" :id="`section-${section.id}`" :key="section.id" :data-section="section.id" :class="{ 'intro-section': activeArticle.contentVersion === 2 && section.position === 0, 'sub-section': (section.level ?? 2) > 2 }">
              <div class="section-heading"><span>{{ String(index + 1).padStart(2, '0') }}</span><component :is="(section.level ?? 2) > 2 ? 'h3' : 'h2'">{{ section.title }}</component><button v-if="section.content" class="section-annotate" :aria-label="`批注${section.title}`" @click="annotateSection(section.content)">批注</button></div>
              <WikiContent v-if="section.contentHtml" :html="section.contentHtml" />
              <p v-else class="plain-section">{{ section.content }}</p>
              <div v-if="section.formula" class="formula-card"><code>{{ section.formula }}</code><button title="复制公式" @click="copyText(section.formula)">复制</button></div>
              <button v-if="articleAnnotations.some(a => section.content.includes(a.quote))" class="margin-note" :aria-label="`查看${section.title}关联批注`" title="查看关联批注" @click="showSectionNotes(section.content)">
                <span v-html="icon('comment', 14)" />{{ articleAnnotations.filter(a => section.content.includes(a.quote)).length }}
              </button>
            </section>
            <div v-if="!activeArticle.sections.some(s => s.content.trim())" class="coming-soon"><span>暂无可展示的正文</span><p>请对照原始词条，或提交内容问题反馈。</p><button @click="openDialog('feedback')">反馈问题</button></div>
            <footer class="article-footer"><span>{{ activeArticle.sourceUrl ? '来源：中文维基参考资料' : '社区投稿 · ' + articleReviewLabel(activeArticle.reviewStatus) }}</span><button @click="openDialog('feedback')">提出补充 / 纠错</button><a v-if="activeArticle.sourceUrl" :href="activeArticle.sourceUrl" target="_blank" rel="noreferrer" v-html="icon('external', 14) + ' 查看原始词条'" /><button v-else @click="panelTab = 'info'; rightOpen = true">查看条目信息</button></footer>
            <ArticleComments :key="activeArticle.id" :article-id="activeArticle.id" :user="user" :online="apiState === 'online'" @login="requestLogin" />
          </article>
        </div>
        <div v-else class="reader-empty empty-state"><span v-html="icon('library', 32)" /><strong>{{ loadingList ? '正在加载条目' : '当前筛选下没有可阅读的条目' }}</strong><small>选择其他分类，或添加一篇中文教学条目。</small><button class="primary-btn" @click="openDialog('article')">新建条目</button></div>
      </main>

      <aside v-if="readerVisible" class="insight-pane" :class="{ closed: !rightOpen }">
        <div class="insight-tabs">
          <button :class="{ active: panelTab === 'ai' }" @click="panelTab = 'ai'"><span v-html="icon('spark', 16)" />AI 解释</button>
          <button :class="{ active: panelTab === 'notes' }" @click="panelTab = 'notes'; noteFilter = ''"><span v-html="icon('comment', 16)" />批注 <em>{{ articleAnnotations.length }}</em></button>
          <button :class="{ active: panelTab === 'info' }" @click="panelTab = 'info'"><span v-html="icon('info', 16)" />条目信息</button>
          <button class="close-panel" aria-label="关闭右侧面板" @click="rightOpen = false" v-html="icon('close', 15)" />
        </div>

        <div v-if="panelTab === 'ai'" class="insight-content ai-panel">
          <div class="ai-intro"><div class="ai-orb">✦</div><div><strong>个性化解释</strong><p>基于当前词条提供解释与引用</p></div></div>
          <p v-if="!capabilities.aiConfigured" class="service-notice">尚未配置模型密钥；发送后仅展示词条引用示例，不会生成 AI 答案。</p>
          <div class="control-grid">
            <label><span>我的程度</span><select v-model="level"><option>小学生</option><option>高中生</option><option>本科生</option><option>专业学习者</option></select></label>
            <label><span>解释方式</span><select v-model="explainStyle"><option>直觉 + 严谨</option><option>生活类比</option><option>公式推导</option><option>例题演示</option></select></label>
          </div>
          <div v-if="conversation.length" class="conversation-toolbar"><span>当前词条对话 · {{ conversation.length }} 轮</span><button class="text-btn" @click="clearConversation">开始新对话</button></div>
          <div class="conversation-thread" aria-live="polite" aria-relevant="additions text">
            <article v-for="(turn, turnIndex) in conversation" :key="turnIndex" class="conversation-turn">
              <p class="user-question">{{ turn.question }}</p>
              <div v-if="turn.result" class="answer-card">
                <div v-if="turn.result.kind !== 'conversation'" class="answer-head"><span class="verified-badge">{{ turn.result.mode === 'demo' ? '引用示例 · 未调用模型' : '模型自检完成' }}</span></div>
                <p class="answer-summary">{{ turn.result.summary }}</p>
                <div class="answer-steps"><div v-for="(step, i) in turn.result.steps" :key="i"><b>{{ i + 1 }}</b><section><strong>{{ step.title }}</strong><p>{{ step.content }}</p></section></div></div>
                <details v-if="turn.result.citations.length"><summary>依据与核验 <span>{{ turn.result.citations.length }} 条引用</span></summary><div class="citation" v-for="(cite, i) in turn.result.citations" :key="i"><span>{{ cite.section }}</span><p>“{{ cite.quote }}”</p></div></details>
                <div v-if="turn.result.kind !== 'conversation'" class="answer-actions"><span>这条解释有帮助吗？</span><button @click="sendFeedback('helpful')">有帮助</button><button @click="sendFeedback('needs_improvement')">需改进</button></div>
              </div>
              <template v-else>
                <p v-if="turn.text" class="stream-answer">{{ turn.text }}</p>
                <p v-if="explaining && turnIndex === conversation.length - 1" class="stream-status" role="status">{{ verificationStage === 3 ? '正在核验引用与结论…' : turn.text ? '正在回答…' : '正在连接模型…' }}</p>
                <p v-if="turn.error" class="inline-error" role="alert">{{ turn.error }}</p>
              </template>
            </article>
          </div>
          <div class="prompt-box" :class="{ listening }">
            <textarea v-model="explainPrompt" rows="3" :placeholder="conversation.length ? '继续追问，例如：第二步能举个例子吗？' : '关于这个词条，你想理解什么？'" @keydown="handleComposerKey($event, runExplain)" />
            <div><button class="mic-btn" :class="{ recording: listening }" title="语音输入" aria-label="语音输入" @click="toggleListening" v-html="icon('mic', 17)" /><span v-if="listening" class="listening-label">正在听…</span><span v-else>Enter 发送 · Ctrl + Enter 换行</span><button v-if="explaining" class="text-btn" @click="stopExplanation">停止生成</button><button v-else class="send-btn" aria-label="发送解释请求" :disabled="!explainPrompt.trim()" @click="runExplain" v-html="icon('send', 16)" /></div>
          </div>
          <div v-if="!conversation.length" class="suggestion-area"><span>可以这样问</span><button @click="explainPrompt = '用一个生活中的例子解释这个算法'; runExplain()">用生活例子解释</button><button @click="explainPrompt = '一步步展示算法如何运行'; runExplain()">一步步演示</button><button @click="explainPrompt = '这个算法最容易误解的地方是什么？'; runExplain()">指出常见误区</button></div>
        </div>

        <div v-else-if="panelTab === 'notes'" class="insight-content notes-panel">
          <div v-if="composerOpen" class="note-composer">
            <div class="selected-quote"><span v-html="icon('quote', 14)" />{{ selectedQuote }}</div>
            <textarea class="annotation-input" v-model="annotationText" rows="4" placeholder="围绕这句话写下你的理解或疑问…" />
            <div><button class="text-btn" @click="composerOpen = false; selectedQuote = ''">取消</button><button class="primary-btn" :disabled="!annotationText.trim() || annotationBusy" @click="saveAnnotation">{{ annotationBusy ? '正在发布…' : '发布批注' }}</button></div>
          </div>
          <div class="notes-header"><div><strong>句子讨论</strong><span>批注与原文保持关联</span></div><select v-model="noteSort" aria-label="批注排序"><option>最相关</option><option>最新</option></select></div>
          <button v-if="noteFilter" class="text-btn" @click="noteFilter = ''">正在查看本段批注 · 显示全部</button>
          <div v-for="note in visibleNotes" :key="note.id" class="note-card">
            <div class="quote-strip" :style="{ borderColor: note.color }"><span v-html="icon('quote', 13)" /><p>{{ note.quote }}</p></div>
            <div class="note-author"><UserAvatar :src="note.avatar" :name="note.author" /><div><strong>{{ note.author }}</strong><small>{{ formatDate(note.createdAt) }}</small></div><button aria-label="反馈此批注" title="反馈此批注" @click="reportNote(note)" v-html="icon('more', 15)" /></div>
            <div v-if="note.status && note.status !== 'visible'" class="note-status">{{ note.status === 'pending' ? '待审核 · 仅作者和管理员可见' : '已驳回 · 仅作者和管理员可见' }}</div>
            <button v-if="canDelete(note.userId)" class="text-btn danger" @click="deleteNote(note)">删除批注</button>
            <p class="note-body">{{ note.body }}</p>
            <div v-if="!note.status || note.status === 'visible'" class="note-actions"><button :aria-label="note.liked ? '取消点赞' : '点赞批注'" :aria-pressed="!!note.liked" :class="{ 'is-liked': note.liked }" :disabled="likesBusy.includes(note.id)" @click="likeNote(note)" v-html="icon('heart', 14) + ` ${note.likes}`" /><button :aria-expanded="expandedNotes.includes(note.id)" @click="openReplies(note)" v-html="icon('comment', 14) + ` ${note.replies} 条回复`" /><button @click="openReplies(note)">{{ expandedNotes.includes(note.id) ? '收起' : '回复' }}</button></div>
            <div v-if="expandedNotes.includes(note.id)" class="reply-thread">
              <p v-if="replyBusy.includes(note.id)" role="status">正在处理回复…</p>
              <p v-if="replyErrors[note.id]" class="inline-error" role="alert">{{ replyErrors[note.id] }}</p>
              <div v-for="comment in commentLists[note.id] || []" :key="comment.id" class="reply-item"><UserAvatar :src="comment.avatar" :name="comment.author" :size="22" /> <strong>{{ comment.author }}</strong><small>{{ formatDate(comment.createdAt) }}</small><p>{{ comment.body }}</p><button v-if="canDelete(comment.userId)" class="text-btn danger" @click="deleteReply(note, comment)">删除回复</button></div>
              <p v-if="!commentLists[note.id]?.length && !replyBusy.includes(note.id)" class="muted">暂无回复，写下你的理解吧。</p>
              <form @submit.prevent="submitReply(note)"><textarea v-model="replyDrafts[note.id]" aria-label="回复内容" placeholder="围绕这条批注回复…" maxlength="2000" rows="3" /><button class="primary-btn" :disabled="!replyDrafts[note.id]?.trim() || replyBusy.includes(note.id)" type="submit">发送回复</button></form>
            </div>
          </div>
          <div v-if="!articleAnnotations.length" class="empty-state"><span v-html="icon('comment', 28)" /><strong>还没有句子批注</strong><small>选中正文中的文字即可发起讨论</small></div>
        </div>

        <div v-else class="insight-content info-panel">
          <h3>条目信息</h3><dl><dt>标题</dt><dd>{{ activeArticle.title }}</dd><dt>学科</dt><dd>{{ activeArticle.category }}</dd><dt>类型</dt><dd>{{ activeArticle.kind }}</dd><dt>难度</dt><dd>{{ activeArticle.difficulty }}</dd><dt>标签</dt><dd><span v-for="tag in activeArticle.tags" :key="tag" class="tag">{{ tag }}</span></dd><dt>预计阅读</dt><dd>{{ activeArticle.readMinutes }} 分钟</dd><dt>许可</dt><dd>{{ activeArticle.license ?? 'CC BY-SA 4.0' }}</dd></dl>
          <div class="quality-box"><span v-html="icon('info', 16)" /><div><strong>{{ activeArticle.sourceUrl ? '中文维基参考资料' : '社区投稿' }}</strong><p>核验状态：{{ articleReviewLabel(activeArticle.reviewStatus) }}。引用与模型自检不代表人工审核通过。</p></div></div>
          <div class="info-management"><button class="text-btn" @click="openDialog('feedback')">提出补充 / 纠错</button><button class="text-btn" @click="openMyFeedback">查看我的反馈</button><template v-if="user?.role === 'admin'"><button class="text-btn" @click="openModeration(activeArticle)">审核此条目</button><button class="text-btn danger" :disabled="articleDeleteBusy" @click="removeActiveArticle">删除条目</button></template></div>
        </div>
      </aside>
    </div>

    <div v-if="selectedQuote && !composerOpen" class="selection-toolbar" :style="{ left: selectionPos.x + 'px', top: selectionPos.y + 'px' }">
      <button @click="explainSelection"><span v-html="icon('spark', 14)" />AI 解释</button><i /><button @click="openAnnotation"><span v-html="icon('comment', 14)" />批注</button>
    </div>
    <div v-if="dialog" class="dialog-backdrop" @click.self="!submitting && (dialog = null)">
      <section class="action-dialog" role="dialog" aria-modal="true" aria-labelledby="dialog-title">
        <header><h2 id="dialog-title">{{ { article: '新建中文条目', category: '新建学科分类', feedback: '提出补充 / 纠错', connection: '连接与功能状态' }[dialog] }}</h2><button class="icon-btn" aria-label="关闭弹窗" :disabled="submitting" @click="dialog = null" v-html="icon('close')" /></header>
        <div v-if="dialog === 'connection'" class="connection-status">
          <p>知识库：{{ apiState === 'online' ? '已连接' : '未连接，正在展示内置样例' }}</p>
          <p>数据库中的已发布条目：{{ stats?.articleCount ?? 0 }} 篇</p>
          <p>数据库类型：{{ stats?.databaseDriver === 'mysql' ? 'MySQL' : stats?.databaseDriver === 'sqlite' ? 'SQLite（开发库）' : '未连接' }}</p>
          <p>AI 模型：{{ capabilities.aiConfigured ? '已配置' : '未配置；仅有引用示例' }}</p>
          <p>云语音识别：尚未接入，当前请使用文字输入</p>
          <p class="muted">当前使用本地演示用户；条目与章节数量来自当前连接的数据库。维基导入条目为机器筛选内容，尚需人工校审。</p>
          <button class="primary-btn" @click="dialog = null; loadBackendContent()">重新连接并刷新</button>
        </div>
        <form v-else @submit.prevent="submitDialog">
          <template v-if="dialog === 'article'">
            <label>条目标题<input v-model="articleDraft.title" required maxlength="255" placeholder="例如：二分查找" /></label>
            <label>学科分类<select v-model="articleDraft.category"><template v-for="subject in subjects" :key="subject.name"><option :value="subject.name">{{ subject.name }}</option><option v-for="child in subject.children" :key="child.name" :value="child.name">{{ child.name }}</option></template></select></label>
            <div class="dialog-columns"><label>条目类型<select v-model="articleDraft.kind"><option>概念</option><option>算法</option><option>例题</option></select></label><label>学习难度<select v-model="articleDraft.difficulty"><option>入门</option><option>进阶</option><option>高级</option></select></label></div>
            <label>条目正文<textarea v-model="articleDraft.content" required maxlength="100000" rows="7" placeholder="用中文描述定义、前提、步骤或例题…" /></label>
            <p class="muted">保存后可在知识库阅读，内容标记为“社区投稿 · 待核验”。提交的原创内容按 CC BY-SA 4.0 分享。</p>
          </template>
          <label v-else-if="dialog === 'category'">分类名称<input v-model="categoryName" required maxlength="80" placeholder="例如：计算几何" /></label>
          <template v-else><p class="muted">建议会提交到管理员的「审核管理 → 纠错反馈」。你可以在左侧「我的反馈」查看处理结果。</p><label>补充或纠错内容<textarea v-model="feedbackText" required maxlength="4000" rows="6" placeholder="指出具体问题，并尽可能提供依据…" /></label></template>
          <p v-if="dialogError" class="inline-error" role="alert">{{ dialogError }}</p>
          <footer><button class="text-btn" type="button" :disabled="submitting" @click="dialog = null">取消</button><button class="primary-btn" type="submit" :disabled="submitting || apiState !== 'online'">{{ submitting ? '正在保存…' : dialog === 'feedback' ? '提交建议' : '保存' }}</button></footer>
          <p v-if="apiState !== 'online'" class="inline-error">后端未连接，暂时无法保存；请使用知识库菜单重新连接。</p>
        </form>
      </section>
    </div>
    <Transition name="toast"><div v-if="toast" class="toast" role="status"><span v-html="icon('info', 15)" />{{ toast.text }}</div></Transition>
  </div>
</template>
