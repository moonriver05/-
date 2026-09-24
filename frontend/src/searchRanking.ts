// Used only for offline demo content. Online results are ranked by the backend
// before pagination, not re-sorted one page at a time in the browser.
type Searchable = { title: string; excerpt: string; tags: string[]; sections: { content: string; title: string }[] }

export function searchRank(article: Searchable, query: string): number {
  const q = query.trim().toLowerCase()
  if (!q) return 0
  const title = article.title.toLowerCase()
  if (title === q) return 0
  if (title.startsWith(q)) return 1
  if (title.includes(q)) return 2
  if ([article.excerpt, ...article.tags].some(value => value.toLowerCase().includes(q))) return 3
  if (article.sections.some(section => [section.title, section.content].some(value => value.toLowerCase().includes(q)))) return 4
  return Infinity
}

export function searchSortParam(label: string): string {
  return ({ '相关度': 'relevance', '标题': 'title', '难度': 'difficulty' } as Record<string, string>)[label] ?? 'updated'
}
