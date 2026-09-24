import test from 'node:test'
import assert from 'node:assert/strict'
import { searchRank, searchSortParam } from '../src/searchRanking.ts'

const article = (title, excerpt = '', tags = [], content = '') => ({ title, excerpt, tags, sections: [{ title: '正文', content }] })

test('exact title, prefix, contained title, summary/tags, then body', () => {
  const rows = [article('机器学习', '', [], '梯度下降 '.repeat(500)), article('优化', '梯度下降'), article('随机梯度下降'), article('梯度下降法'), article('梯度下降')]
  assert.deepEqual(rows.sort((a, b) => searchRank(a, '梯度下降') - searchRank(b, '梯度下降')).map(a => a.title), ['梯度下降', '梯度下降法', '随机梯度下降', '优化', '机器学习'])
  assert.equal(searchRank(article('其他', '', ['梯度下降']), '梯度下降'), 3)
  assert.equal(searchRank(article('其他'), '梯度下降'), Infinity)
})

test('trimmed case-insensitive names, literal wildcard characters, explicit sort modes', () => {
  assert.equal(searchRank(article('Dijkstra'), ' dIJKSTRA '), 0)
  assert.equal(searchRank(article('其他'), '%'), Infinity)
  assert.equal(searchRank(article('a_b'), '_'), 2)
  assert.equal(searchRank(article('任意'), ' '), 0)
  assert.equal(searchSortParam('相关度'), 'relevance')
  assert.equal(searchSortParam('最近更新'), 'updated')
  assert.equal(searchSortParam('标题'), 'title')
  assert.equal(searchSortParam('难度'), 'difficulty')
})
