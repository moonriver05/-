package seed

import (
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mathwiki/internal/model"
)

type demoSection struct {
	title   string
	content string
	formula string
}

type demoArticle struct {
	pageID      int64
	slug        string
	title       string
	lead        string
	category    string
	tags        []string
	kind        string
	difficulty  string
	readMinutes int
	sections    []demoSection
}

// Ensure creates the fixed MVP user and, for an empty database, a small Chinese
// teaching corpus. A later core-articles.jsonl import can coexist with it.
func Ensure(db *gorm.DB, withArticles bool) error {
	user := model.User{ID: 1, Name: "演示用户", Role: "learner"}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&user).Error; err != nil {
		return err
	}
	if !withArticles {
		return nil
	}

	var count int64
	if err := db.Model(&model.Article{}).Where("published = ?", true).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var dijkstraID uint
		for index, item := range demoCorpus() {
			plainParts := make([]string, 0, len(item.sections)+1)
			plainParts = append(plainParts, item.lead)
			for _, section := range item.sections {
				plainParts = append(plainParts, section.content)
			}
			article := model.Article{
				PageID:       item.pageID,
				RevisionID:   1,
				Title:        item.title,
				SourceTitle:  item.title,
				Slug:         item.slug,
				Lead:         item.lead,
				PlainText:    strings.Join(plainParts, "\n\n"),
				Category:     item.category,
				Categories:   jsonList([]string{item.category}),
				Tags:         jsonList(item.tags),
				Kind:         item.kind,
				Difficulty:   item.difficulty,
				ReadMinutes:  item.readMinutes,
				Language:     "zh-Hans",
				SourceURL:    "https://zh.wikipedia.org/wiki/" + item.title,
				License:      "CC BY-SA 4.0",
				ReviewStatus: "demo_verified",
				Published:    true,
				CreatedAt:    time.Now().Add(-time.Duration(index+1) * 24 * time.Hour),
				UpdatedAt:    time.Now().Add(-time.Duration(index) * 6 * time.Hour),
			}
			if err := tx.Create(&article).Error; err != nil {
				return err
			}
			for position, source := range item.sections {
				formulae := "[]"
				if source.formula != "" {
					formulae = jsonList([]string{source.formula})
				}
				section := model.Section{ArticleID: article.ID, Position: position + 1, Title: source.title, Content: source.content, Formulae: formulae}
				if err := tx.Create(&section).Error; err != nil {
					return err
				}
			}
			if item.slug == "dijkstra" {
				dijkstraID = article.ID
			}
		}

		if dijkstraID == 0 {
			return nil
		}
		if err := tx.Create(&model.Favorite{UserID: 1, ArticleID: dijkstraID}).Error; err != nil {
			return err
		}
		annotations := []model.Annotation{
			{ArticleID: dijkstraID, UserID: 1, Quote: "由于所有边权非负", Body: "这里是整个贪心证明的关键。如果允许负权边，后续路径就可能把已经确认的距离再次变小。", Color: "#f6c85f", Status: "visible", Likes: 12, CreatedAt: time.Now().Add(-18 * time.Minute)},
			{ArticleID: dijkstraID, UserID: 1, Quote: "水波从源点向外传播", Body: "这个比喻很直观。可以补充：边权对应水流经过管道所需的时间。", Color: "#7eb6e8", Status: "visible", Likes: 8, CreatedAt: time.Now().Add(-time.Hour)},
		}
		return tx.Create(&annotations).Error
	})
}

func jsonList(values []string) string {
	encoded, _ := json.Marshal(values)
	return string(encoded)
}

func demoCorpus() []demoArticle {
	return []demoArticle{
		{
			pageID: 90000001, slug: "dijkstra", title: "Dijkstra 算法", category: "图论 · 最短路径", tags: []string{"图论", "贪心", "最短路径"}, kind: "算法", difficulty: "进阶", readMinutes: 12,
			lead: "求解带非负权边的图中，单一源点到其他所有顶点的最短路径。",
			sections: []demoSection{
				{title: "核心思想", content: "Dijkstra 算法维护一个已经确定最短距离的顶点集合。每一步选择当前未确定顶点中距离源点最近的一个，并用它去松弛相邻边。由于所有边权非负，一旦一个顶点成为当前最近点，它的最短距离就不会再被后来经过的路径改小。"},
				{title: "松弛操作", content: "设当前顶点为 u，它的邻接点为 v，边权为 w。如果从源点先到 u 再经过边 (u, v) 比当前记录的距离更短，就更新 v 的距离。", formula: "dist[v] = min(dist[v], dist[u] + w(u,v))"},
				{title: "正确性直觉", content: "可以把路径扩张想象成水波从源点向外传播。非负边权保证水波不会穿过一条边后回到更早的时间；因此最先被触达并确认的未访问顶点，已经取得全局最短距离。"},
				{title: "复杂度与限制", content: "使用邻接表和二叉堆时，时间复杂度为 O((V+E) log V)，通常写作 O(E log V)。如果图中存在负权边，这个贪心前提不再成立，应改用 Bellman–Ford 等算法。"},
			},
		},
		{
			pageID: 90000002, slug: "gradient-descent", title: "梯度下降", category: "优化算法", tags: []string{"优化", "机器学习"}, kind: "算法", difficulty: "入门", readMinutes: 9,
			lead:     "沿目标函数梯度的反方向迭代移动，以寻找函数的局部极小值。",
			sections: []demoSection{{title: "基本定义", content: "梯度指出函数增长最快的方向，因此沿梯度的反方向移动会使函数值在局部下降。学习率决定每次移动的步长。", formula: "xₜ₊₁ = xₜ − η∇f(xₜ)"}},
		},
		{
			pageID: 90000003, slug: "dynamic-programming", title: "动态规划", category: "算法设计", tags: []string{"DP", "递推"}, kind: "算法", difficulty: "进阶", readMinutes: 15,
			lead:     "将具有重叠子问题和最优子结构的问题分解，并保存子问题答案。",
			sections: []demoSection{{title: "两个关键性质", content: "动态规划常用于同时具备最优子结构与重叠子问题的问题。状态定义决定了我们保存什么，状态转移描述了已知答案如何推出未知答案。"}},
		},
		{
			pageID: 90000004, slug: "bayes-theorem", title: "贝叶斯定理", category: "概率论", tags: []string{"概率", "统计"}, kind: "概念", difficulty: "入门", readMinutes: 7,
			lead:     "描述已知某些条件后，如何更新事件发生概率的基本定理。",
			sections: []demoSection{{title: "条件概率", content: "贝叶斯定理把后验概率写成似然与先验概率的乘积，再除以证据概率。", formula: "P(A|B) = P(B|A)P(A) / P(B)"}},
		},
		{
			pageID: 90000005, slug: "fft", title: "快速傅里叶变换", category: "数值计算", tags: []string{"信号处理", "分治"}, kind: "算法", difficulty: "高级", readMinutes: 18,
			lead:     "高效计算离散傅里叶变换及其逆变换的一类算法。",
			sections: []demoSection{{title: "分治结构", content: "FFT 利用单位根的对称性与周期性，把长度为 n 的变换拆成奇数项和偶数项两个长度为 n/2 的变换。"}},
		},
	}
}
