import type { Annotation, Article, Explanation, Stats, Category, Comment, Capabilities, ArticleInput, User, FeedbackItem } from './types'

type AnnotationInput = {
  articleId: number
  sectionId?: number
  quote: string
  body: string
  color?: string
}

type ExplainInput = {
  articleId: number
  question: string
  audience: string
  style: string
}

export class ApiError extends Error {
  status: number
  retryAfter: number

  constructor(message: string, status: number, retryAfter = 0) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.retryAfter = retryAfter
  }
}

async function request<T>(path: string, init: RequestInit = {}, timeout = 10000): Promise<T> {
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), timeout)
  try {
    const response = await fetch(`/api/v1${path}`, {
      ...init,
      credentials: 'same-origin',
      signal: controller.signal,
      headers: { ...(init.body instanceof FormData ? {} : { 'Content-Type': 'application/json' }), 'X-MathWiki-Request': '1', ...init.headers }
    })
    if (!response.ok) {
      if (response.status === 401 && (!path.startsWith('/auth/') || path.startsWith('/auth/profile') || path === '/auth/avatar')) window.dispatchEvent(new Event('mathwiki:login-required'))
      const payload = await response.json().catch(() => ({})) as { error?: string }
      throw new ApiError(payload.error || `请求失败（${response.status}）`, response.status, Number(response.headers.get('Retry-After')) || 0)
    }
    return await response.json() as T
  } catch (error) {
    if (error instanceof DOMException && error.name === 'AbortError') throw new Error('请求超时，请稍后重试')
    throw error
  } finally {
    window.clearTimeout(timer)
  }
}

async function transcribeAudio(audio: Blob): Promise<{ text: string; provider: string }> {
  const controller = new AbortController()
  const timer = window.setTimeout(() => controller.abort(), 30_000)
  const form = new FormData()
  const extension = audio.type.includes('ogg') ? 'ogg' : audio.type.includes('mp4') ? 'm4a' : 'webm'
  form.append('audio', audio, `recording.${extension}`)
  try {
    const response = await fetch('/api/v1/speech/transcribe', { method: 'POST', headers: { 'X-MathWiki-Request': '1' }, credentials: 'same-origin', body: form, signal: controller.signal })
    if (!response.ok) {
      const payload = await response.json().catch(() => ({})) as { error?: string }
      throw new ApiError(payload.error || `转写失败（${response.status}）`, response.status)
    }
    return await response.json() as { text: string; provider: string }
  } finally {
    window.clearTimeout(timer)
  }
}

export const api = {
  adminUsers: (page = 1, q = '') => request<{ items: User[]; total: number }>(`/admin/users?${new URLSearchParams({ page: String(page), q })}`),
  banUser: (id: number, banned: boolean, reason: string) => request(`/admin/users/${id}`, { method: 'PATCH', body: JSON.stringify({ banned, reason }) }),
  adminAudits: (page = 1) => request<{ items: { id: number; userId: number; action: string; target: string; reason: string; status: number; createdAt: string }[]; total: number }>(`/admin/audits?page=${page}`),
  sendResetCode: (email: string) => request<{ message: string; retryAfter: number; expiresIn: number }>('/auth/reset-code', { method: 'POST', body: JSON.stringify({ email }) }, 20000),
  resetPassword: (email: string, code: string, password: string) => request<{ message: string }>('/auth/reset-password', { method: 'POST', body: JSON.stringify({ email, code, password }) }),
  adminArticles: (status: string, page = 1, q = '') => request<{ items: Article[]; total: number }>(`/admin/articles?${new URLSearchParams({ status, page: String(page), q })}`),
  adminArticle: (id: number) => request<Article>(`/admin/articles/${id}`),
  reviewArticle: (article: Article, status: 'approved' | 'rejected') => request(`/admin/articles/${article.id}`, { method: 'PATCH', body: JSON.stringify({ status, expectedStatus: article.reviewStatus }) }),
  deleteArticle: (id: number) => request(`/admin/articles/${id}`, { method: 'DELETE' }),
  restoreArticle: (id: number) => request(`/admin/articles/${id}/restore`, { method: 'POST' }),
  adminFeedback: (status: string, page = 1) => request<{ items: FeedbackItem[]; total: number }>(`/admin/feedback?status=${status}&page=${page}`),
  myFeedback: (status = 'all', page = 1) => request<{ items: FeedbackItem[]; total: number }>(`/feedback/mine?status=${status}&page=${page}`),
  reviewFeedback: (id: number, status: 'resolved' | 'dismissed', resolution: string) => request(`/admin/feedback/${id}`, { method: 'PATCH', body: JSON.stringify({ status, resolution }) }),
  profileConfig: () => request<{ avatarConfigured: boolean }>('/auth/profile-config'),
  updateProfile: (input: { name: string; bio: string; location: string; website: string }) => request<{ user: User }>('/auth/profile', { method: 'PUT', body: JSON.stringify(input) }),
  uploadAvatar: (image: Blob) => {
    const body = new FormData(); body.append('avatar', image, 'avatar.jpg')
    return request<{ user: User }>('/auth/avatar', { method: 'PUT', body }, 30000)
  },
  resetAvatar: () => request<{ user: User }>('/auth/avatar', { method: 'DELETE' }),
  me: () => request<{ user: User | null }>('/auth/me'),
  login: (input: { username: string; password: string }) => request<{ user: User }>('/auth/login', { method: 'POST', body: JSON.stringify(input) }),
  register: (input: { username: string; name: string; password: string; email: string; code: string }) => request<{ user: User }>('/auth/register', { method: 'POST', body: JSON.stringify(input) }),
  authConfig: () => request<{ emailVerificationRequired: boolean; emailConfigured: boolean }>('/auth/config'),
  sendEmailCode: (email: string) => request<{ message: string; retryAfter: number; expiresIn: number }>('/auth/email-code', { method: 'POST', body: JSON.stringify({ email }) }, 20000),
  logout: () => request('/auth/logout', { method: 'POST' }),
  articleComments: (id: number, page = 1) => request<{ items: Comment[]; total: number }>(`/articles/${id}/comments?page=${page}`),
  postArticleComment: (id: number, body: string) => request<Comment>(`/articles/${id}/comments`, { method: 'POST', body: JSON.stringify({ body }) }),
  deleteArticleComment: (id: number) => request(`/article-comments/${id}`, { method: 'DELETE' }),
  deleteComment: (id: number) => request(`/comments/${id}`, { method: 'DELETE' }),
  deleteAnnotation: (id: number) => request(`/annotations/${id}`, { method: 'DELETE' }),
  moderationQueue: (status = 'pending', page = 1) => request<{ items: Annotation[]; total: number }>(`/admin/annotations?status=${status}&page=${page}`),
  reviewAnnotation: (id: number, status: 'visible' | 'rejected') => request(`/admin/annotations/${id}`, { method: 'PATCH', body: JSON.stringify({ status }) }),
  getCategories: () => request<Category[]>('/categories'),
  createCategory: (name: string) => request('/categories', { method: 'POST', body: JSON.stringify({ name }) }),
  getCapabilities: () => request<Capabilities>('/capabilities'),
  listArticles: (params: Record<string, string>) => request<{ items: Article[]; total: number }>(`/articles?${new URLSearchParams({ ...params, format: 'page', limit: '50' })}`),
  createArticle: (input: ArticleInput) => request<Article>('/articles', { method: 'POST', body: JSON.stringify(input) }),
  markRead: (id: number) => request(`/articles/${id}/read`, { method: 'PUT', body: '{}' }),
  getComments: (id: number) => request<Comment[]>(`/annotations/${id}/comments`),
  createComment: (id: number, body: string) => request<Comment>(`/annotations/${id}/comments`, { method: 'POST', body: JSON.stringify({ body }) }),
  getStats: () => request<Stats>('/stats'),
  getArticles: (limit = 100) => request<Article[]>(`/articles?limit=${limit}`),
  getArticle: (slug: string) => request<Article>(`/articles/${encodeURIComponent(slug)}`),
  search: (query: string) => request<Article[]>(`/search?q=${encodeURIComponent(query)}&limit=50`),
  getAnnotations: (articleId: number) => request<Annotation[]>(`/articles/${articleId}/annotations`),
  createAnnotation: (input: AnnotationInput) => request<Annotation>('/annotations', { method: 'POST', body: JSON.stringify(input) }),
  setFavorite: (articleId: number, saved: boolean) => request<{ articleId: number; saved: boolean }>(`/articles/${articleId}/favorite`, { method: 'PUT', body: JSON.stringify({ saved }) }),
  likeAnnotation: (annotationId: number, liked = true) => request<{ id: number; likes: number; liked: boolean }>(`/annotations/${annotationId}/like`, { method: liked ? 'POST' : 'DELETE', body: '{}' }),
  explain: (input: ExplainInput) => request<Explanation>('/explanations', { method: 'POST', body: JSON.stringify(input) }, 190_000),
  feedback: (articleId: number, kind: string, body = '') => request('/feedback', { method: 'POST', body: JSON.stringify({ articleId, kind, body }) }),
  transcribe: transcribeAudio
}
