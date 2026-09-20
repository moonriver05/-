import type { Annotation, Article, Explanation } from './types'

export const articles: Article[] = [
  {
    id: 1, slug: 'dijkstra', title: 'Dijkstra 算法', category: '图论 · 最短路径',
    excerpt: '求解带非负权边的图中，单一源点到其他所有顶点的最短路径。',
    tags: ['图论', '贪心', '最短路径'], kind: '算法', difficulty: '进阶', readMinutes: 12, updatedAt: '今天 14:32', saved: true,
    sections: [
      { id: 11, title: '核心思想', content: 'Dijkstra 算法维护一个已经确定最短距离的顶点集合。每一步选择当前未确定顶点中距离源点最近的一个，并用它去松弛相邻边。由于所有边权非负，一旦一个顶点成为当前最近点，它的最短距离就不会再被后来经过的路径改小。' },
      { id: 12, title: '松弛操作', content: '设当前顶点为 u，它的邻接点为 v，边权为 w。如果从源点先到 u 再经过边 (u, v) 比当前记录的距离更短，就更新 v 的距离。', formula: 'dist[v] = min(dist[v], dist[u] + w(u,v))' },
      { id: 13, title: '正确性直觉', content: '可以把路径扩张想象成水波从源点向外传播。非负边权保证水波不会穿过一条边后回到更早的时间；因此最先被触达并确认的未访问顶点，已经取得全局最短距离。' },
      { id: 14, title: '复杂度与限制', content: '使用邻接表和二叉堆时，时间复杂度为 O((V+E) log V)，通常写作 O(E log V)。如果图中存在负权边，这个贪心前提不再成立，应改用 Bellman–Ford 等算法。' }
    ]
  },
  {
    id: 2, slug: 'gradient-descent', title: '梯度下降', category: '优化算法',
    excerpt: '沿目标函数梯度的反方向迭代移动，以寻找函数的局部极小值。',
    tags: ['优化', '机器学习'], kind: '算法', difficulty: '入门', readMinutes: 9, updatedAt: '昨天', saved: true,
    sections: [{ id: 21, title: '基本定义', content: '梯度指出函数增长最快的方向，因此沿梯度的反方向移动会使函数值在局部下降。学习率决定每次移动的步长。', formula: 'xₜ₊₁ = xₜ − η∇f(xₜ)' }]
  },
  {
    id: 3, slug: 'dynamic-programming', title: '动态规划', category: '算法设计',
    excerpt: '将具有重叠子问题和最优子结构的问题分解，并保存子问题答案。',
    tags: ['DP', '递推'], kind: '算法', difficulty: '进阶', readMinutes: 15, updatedAt: '9 月 12 日', saved: false,
    sections: [{ id: 31, title: '两个关键性质', content: '动态规划常用于同时具备最优子结构与重叠子问题的问题。状态定义决定了我们保存什么，状态转移描述了已知答案如何推出未知答案。' }]
  },
  {
    id: 4, slug: 'bayes-theorem', title: '贝叶斯定理', category: '概率论',
    excerpt: '描述已知某些条件后，如何更新事件发生概率的基本定理。',
    tags: ['概率', '统计'], kind: '概念', difficulty: '入门', readMinutes: 7, updatedAt: '9 月 10 日', saved: true,
    sections: [{ id: 41, title: '条件概率', content: '贝叶斯定理把后验概率写成似然与先验概率的乘积，再除以证据概率。', formula: 'P(A|B) = P(B|A)P(A) / P(B)' }]
  },
  {
    id: 5, slug: 'fft', title: '快速傅里叶变换', category: '数值计算',
    excerpt: '高效计算离散傅里叶变换及其逆变换的一类算法。',
    tags: ['信号处理', '分治'], kind: '算法', difficulty: '高级', readMinutes: 18, updatedAt: '9 月 8 日', saved: false,
    sections: [{ id: 51, title: '分治结构', content: 'FFT 利用单位根的对称性与周期性，把长度为 n 的变换拆成奇数项和偶数项两个长度为 n/2 的变换。' }]
  }
]

export const seedAnnotations: Annotation[] = [
  { id: 1, articleId: 1, quote: '由于所有边权非负', body: '这里是整个贪心证明的关键。如果允许负权边，后续路径就可能把已经确认的距离再次变小。', author: '林序', color: '#f6c85f', createdAt: '18 分钟前', replies: 4, likes: 12 },
  { id: 2, articleId: 1, quote: '水波从源点向外传播', body: '这个比喻很直观。可以补充：边权对应水流经过管道所需的时间。', author: 'Mori', color: '#7eb6e8', createdAt: '1 小时前', replies: 2, likes: 8 }
]

export function mockExplain(level: string, focus: string): Explanation {
  return {
    summary: level === '小学生' ? '把它想成在地图上找一条最快到达各个地方的路线。' : 'Dijkstra 用贪心策略逐步固定源点到各顶点的最短距离，前提是边权非负。',
    steps: [
      { title: '先记下暂定距离', content: '源点记为 0，其余顶点先记为无穷大。它们表示目前已知的最好路线。' },
      { title: '确认最近的点', content: focus || '从尚未确认的顶点中，取暂定距离最小的一个；非负边权保证它已经不可能被绕路改进。' },
      { title: '向邻居传播', content: '尝试经由刚确认的顶点到达每个邻居，如果更短，就更新邻居的暂定距离。' }
    ],
    citations: [
      { section: '核心思想 · 第 1 段', quote: '每一步选择当前未确定顶点中距离源点最近的一个' },
      { section: '复杂度与限制 · 第 1 段', quote: '如果图中存在负权边，这个贪心前提不再成立' }
    ],
    confidence: 0.96,
    checks: ['非负边权前提已确认', '复杂度表述与正文一致', '2 条结论可回溯到当前词条']
  }
}
