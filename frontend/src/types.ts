export type Section = { id: number; title: string; content: string; formula?: string; contentHtml?: string; level?: number; position?: number }
export type Article = {
  published?: boolean
  id: number
  slug: string
  title: string
  excerpt: string
  category: string
  tags: string[]
  kind: '概念' | '算法' | '例题'
  difficulty: '入门' | '进阶' | '高级'
  readMinutes: number
  updatedAt: string
  saved: boolean
  sections: Section[]
  pageId?: number
  sourceUrl?: string
  license?: string
  reviewStatus?: string
  contentVersion?: number
  contentWarnings?: string[]
}
export type Annotation = {
  avatar?: string
  userId?: number
  status?: 'pending' | 'visible' | 'rejected'
  id: number
  articleId: number
  quote: string
  body: string
  author: string
  color: string
  createdAt: string
  replies: number
  likes: number
  liked?: boolean
}
export type Explanation = {
  mode?: 'demo' | 'model'
  summary: string
  steps: { title: string; content: string }[]
  citations: { section: string; quote: string }[]
  confidence: number
  checks: string[]
}

export type Stats = {
  databaseDriver: string
  articleCount: number
  sectionCount: number
  annotationCount: number
  favoriteCount: number
  recentCount: number
  language: string
  categories: { category: string; count: number }[]
}

export type Category = { name: string; count: number; children: Category[] }
export type Comment = { id: number; userId: number; body: string; author: string; avatar?: string; createdAt: string }
export type User = { id: number; username: string; name: string; role: 'learner' | 'admin'; banned?: boolean; avatar?: string; bio?: string; location?: string; website?: string }
export type Capabilities = { aiConfigured: boolean; speechConfigured: boolean }
export type ArticleInput = { title: string; category: string; kind: string; difficulty: string; content: string }
export type FeedbackItem = { id: number; userId: number; articleId?: number; articleTitle: string; author: string; kind: string; body: string; status: 'pending' | 'resolved' | 'dismissed'; resolution: string; createdAt: string; reviewedAt?: string }
