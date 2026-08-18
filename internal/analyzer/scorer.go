package analyzer

import "github.com/ai-code-review/aicr/internal/domain"

// Score 根据各维度评分、维度权重与 findings 计算综合分。
// 规则：按维度权重加权平均；critical/high 等问题对其 category 对应维度做硬性扣分；
// 存在 critical 时总分封顶 70。评分只作相对指示，不追求精确。
// Pi Agent 未返回某维度时该维度按 0 分处理（不报错）。
func Score(dims map[string]Dimension, specs []domain.DimensionSpec, findings []ReportFinding) int {
	scores := make(map[string]int, len(specs))
	for _, s := range specs {
		if d, ok := dims[s.Key]; ok {
			scores[s.Key] = clampScore(d.Score)
		} else {
			scores[s.Key] = 0
		}
	}

	// 按问题严重度对所属维度扣分
	var hasCritical bool
	for _, f := range findings {
		if _, ok := scores[f.Category]; !ok {
			continue
		}
		switch f.Severity {
		case "critical":
			hasCritical = true
			scores[f.Category] -= 20
			// critical 额外拉低安全维度（若存在）
			if f.Category != "security" {
				if _, ok := scores["security"]; ok {
					scores["security"] -= 10
				}
			}
		case "high":
			scores[f.Category] -= 10
		case "medium":
			scores[f.Category] -= 5
		case "low":
			scores[f.Category] -= 2
		}
	}

	// 权重归一化
	var totalWeight float64
	for _, s := range specs {
		if s.Weight > 0 {
			totalWeight += s.Weight
		}
	}
	var total float64
	if totalWeight > 0 {
		for _, s := range specs {
			if s.Weight > 0 {
				total += float64(clampScore(scores[s.Key])) * (s.Weight / totalWeight)
			}
		}
	} else {
		// 没有有效权重时退化为简单平均
		var sum, n int
		for _, s := range specs {
			sum += clampScore(scores[s.Key])
			n++
		}
		if n > 0 {
			total = float64(sum) / float64(n)
		}
	}

	out := int(total)
	if hasCritical && out > 70 {
		out = 70
	}
	return clampScore(out)
}

func clampScore(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
