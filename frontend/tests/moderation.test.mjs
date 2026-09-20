import test from 'node:test'
import assert from 'node:assert/strict'
import { articleReviewLabel, feedbackKindLabel, feedbackStatusLabel } from '../src/moderation.ts'
test('review labels distinguish imported content from verified articles', () => {
  assert.equal(articleReviewLabel('approved'), '人工审核通过')
  assert.match(articleReviewLabel('community_pending'), /未公开/)
  assert.match(articleReviewLabel('rejected'), /驳回/)
  assert.match(articleReviewLabel('machine_selected'), /待人工核验/)
  assert.match(articleReviewLabel('machine_imported'), /待人工核验/)
})
test('feedback labels expose processing outcome in Chinese', () => {
  assert.equal(feedbackKindLabel('content_suggestion'), '补充 / 纠错')
  assert.equal(feedbackStatusLabel('pending'), '待处理')
  assert.equal(feedbackStatusLabel('resolved'), '已处理')
  assert.equal(feedbackStatusLabel('dismissed'), '不采纳')
})
