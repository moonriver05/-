export type ModerationSection = 'articles' | 'annotations' | 'feedback'
export function articleReviewLabel(status?: string): string {
  return ({ community_pending: '待审核（未公开）', approved: '人工审核通过', rejected: '已驳回（未公开）', machine_selected: '导入内容 · 待人工核验' } as Record<string, string>)[status || ''] || '导入内容 · 待人工核验'
}
export function feedbackKindLabel(kind: string): string {
  return ({ content_suggestion: '补充 / 纠错', helpful: '解释有帮助', needs_improvement: '解释需改进' } as Record<string, string>)[kind] || kind
}
export function feedbackStatusLabel(status: string): string {
  return ({ pending: '待处理', resolved: '已处理', dismissed: '不采纳' } as Record<string, string>)[status] || status
}
