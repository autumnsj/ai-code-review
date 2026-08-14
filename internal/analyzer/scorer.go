package analyzer

// Score 根据四维度评分与 findings 计算综合分。
// 规则：维度加权平均；critical/high 等问题对对应维度做硬性扣分；
// 存在 critical 时总分封顶。评分只作相对指示，不追求精确。
func Score(d Dimensions, findings []ReportFinding) int {
	arch := clampScore(d.Architecture.Score)
	qual := clampScore(d.Quality.Score)
	sec := clampScore(d.Security.Score)
	maint := clampScore(d.Maintainability.Score)

	// 按问题严重度扣分
	var hasCritical bool
	deduct := map[string]*int{
		"architecture": &arch, "quality": &qual, "security": &sec, "maintainability": &maint,
	}
	for _, f := range findings {
		switch f.Severity {
		case "critical":
			hasCritical = true
			if p, ok := deduct[f.Category]; ok {
				*p -= 20
			}
			sec -= 10 // critical 额外拉低安全分
		case "high":
			if p, ok := deduct[f.Category]; ok {
				*p -= 10
			}
		case "medium":
			if p, ok := deduct[f.Category]; ok {
				*p -= 5
			}
		case "low":
			if p, ok := deduct[f.Category]; ok {
				*p -= 2
			}
		}
	}
	arch, qual, sec, maint = clampScore(arch), clampScore(qual), clampScore(sec), clampScore(maint)

	total := int(0.2*float64(arch) + 0.3*float64(qual) + 0.3*float64(sec) + 0.2*float64(maint))
	if hasCritical && total > 70 {
		total = 70
	}
	return clampScore(total)
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
