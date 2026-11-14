package domain

import "strings"

// TaxMark はレシートに印字された記号と税率の対応。Note はその意味を示す原文。
type TaxMark struct {
	Mark string `json:"mark"`
	Rate *int64 `json:"rate"`
	Note string `json:"note"`
}

// TaxInference は逆算の結果。ScoreGap は順位付けの差であり正解確率ではない。
type TaxInference struct {
	Version   string `json:"version"`
	Status    string `json:"status"`
	Reason    string `json:"reason"`
	Solutions int    `json:"solutions"` // 2 は2通り以上
	ScoreGap  int    `json:"score_gap"`
	States    int    `json:"states"`
}

const maxTaxSearchTransitions = 250_000

type taxGroup struct {
	rate, target int64
	mode         string
}
type taxAssignment struct {
	mask  uint64
	score int
}

// InferDetailTaxes は印字根拠を保ち、金額制約を満たす商品別税率を補完する。
// 複数解の順位は提案にだけ使用し、探索未完了や矛盾を確定扱いにしない。
func InferDetailTaxes(r ReceiptReading, e AmountEvidence) (result AmountEvidence) {
	defer func() {
		for i := range result.DetailTaxes {
			if result.DetailTaxes[i].Status == "unresolved" && result.DetailTaxes[i].Reason == "insufficient_evidence" {
				result.DetailTaxes[i].Reason = result.Inference.Reason
			}
		}
	}()
	e.TaxMarks = append([]TaxMark(nil), r.TaxMarks...)
	e.Inference = &TaxInference{Version: "v1", Status: "unresolved", Reason: "insufficient_evidence"}
	e.DetailTaxes = make([]DetailTax, len(r.Details))
	conflict := false
	for i, d := range r.Details {
		amount := d.Amount
		t := DetailTax{OriginalName: d.Name, OriginalAmount: &amount, OriginalCategory: d.Category, OriginalQuantity: d.Quantity, Rate: d.TaxRate, Mode: d.TaxMode, OriginalRate: d.TaxRate, OriginalMode: d.TaxMode, Mark: d.TaxMark, DiscountAmount: d.DiscountAmount, Status: "unresolved", Reason: "insufficient_evidence"}
		if validTaxRate(t.Rate) {
			t.Status, t.Reason = "printed", "printed_rate"
		} else {
			t.Rate = nil
		}
		for _, mark := range r.TaxMarks {
			if strings.TrimSpace(d.TaxMark) == "" || strings.TrimSpace(mark.Mark) != strings.TrimSpace(d.TaxMark) || strings.TrimSpace(mark.Note) == "" || !validTaxRate(mark.Rate) {
				continue
			}
			if t.Rate != nil && *t.Rate != *mark.Rate {
				conflict = true
			}
			t.Rate, t.Status, t.Reason = mark.Rate, "printed", "printed_mark"
		}
		e.DetailTaxes[i] = t
	}
	if conflict {
		return rejectTaxInference(e, "conflicting_marks")
	}
	if len(r.Details) == 0 || len(r.Details) > maxDetails || e.Selected == nil {
		return e
	}
	resolvedTaxes := resolveTaxModes(r, e.Selected.Amount)
	e.Taxes = resolvedTaxes
	resolvedReading := r
	resolvedReading.TaxBreakdown = resolvedTaxes
	e.TaxMode = taxMode(resolvedReading)
	groups, reason := inferenceGroups(resolvedTaxes)
	if reason != "" {
		if reason == "inconsistent_breakdown" {
			return rejectTaxInference(e, reason)
		}
		e.Inference.Reason = reason
		return e
	}
	var total, assignedDiscount int64
	amounts := make([]int64, len(r.Details))
	for i, d := range r.Details {
		if d.Amount < 0 || d.Amount > 10_000_000 || strings.TrimSpace(d.Name) == "" {
			return rejectTaxInference(e, "invalid_detail")
		}
		discount := int64(0)
		if d.DiscountAmount != nil {
			discount = *d.DiscountAmount
		}
		if discount < 0 || discount > d.Amount {
			return rejectTaxInference(e, "invalid_discount")
		}
		amounts[i] = d.Amount - discount
		total += amounts[i]
		assignedDiscount += discount
	}
	discount := receiptDiscount(e.Candidates)
	if assignedDiscount > discount {
		return rejectTaxInference(e, "inconsistent_discount")
	}
	unassignedDiscount := discount - assignedDiscount
	var target, gross int64
	for i, g := range groups {
		target += g.target
		gross += g.target
		if g.mode == "external" {
			gross += *r.TaxBreakdown[i].TaxAmount
		}
	}
	if total-unassignedDiscount != target || gross != e.Selected.Amount {
		return rejectTaxInference(e, "amount_mismatch")
	}
	if unassignedDiscount > 0 && len(groups) > 1 {
		e.Inference.Reason = "unassigned_discount"
		return e
	}
	// 単一税率なら値引きの所属は自明。商品別への値引き配分は行わない。
	if len(groups) == 1 {
		groups[0].target += unassignedDiscount
	}
	for i, d := range r.Details {
		if d.TaxMode != "" && d.TaxMode != "unknown" && d.TaxMode != groups[0].mode {
			return rejectTaxInference(e, "conflicting_mode")
		}
		t := e.DetailTaxes[i]
		if t.Rate != nil {
			found := false
			for _, g := range groups {
				found = found || *t.Rate == g.rate
			}
			if !found {
				return rejectTaxInference(e, "conflicting_rate")
			}
		}
	}
	assignments, visited, exhausted := searchTaxAssignments(r.Details, amounts, e.DetailTaxes, groups)
	e.Inference.States = visited
	if exhausted {
		e.Inference.Reason = "search_limit"
		return e
	}
	if len(assignments) == 0 {
		return rejectTaxInference(e, "no_solution")
	}
	e.Inference.Solutions = len(assignments)
	e.Inference.Status, e.Inference.Reason = "unique", "amount_constraints"
	if len(assignments) > 1 {
		e.Inference.ScoreGap = assignments[0].score - assignments[1].score
		e.Inference.Status, e.Inference.Reason = "ambiguous", "multiple_solutions"
		if e.Inference.ScoreGap > 0 {
			e.Inference.Status, e.Inference.Reason = "estimated", "product_preference"
		}
	}
	for i := range e.DetailTaxes {
		t := &e.DetailTaxes[i]
		index := 0
		if assignments[0].mask&(uint64(1)<<i) != 0 {
			index = 1
		}
		rate := groups[index].rate
		if len(assignments) == 1 {
			t.Rate, t.Mode = &rate, groups[index].mode
			if t.Status != "printed" {
				t.Status, t.Reason = "reconciled", "amount_constraints"
			}
		} else if t.Rate == nil {
			t.Status, t.Reason = "unresolved", "multiple_solutions"
			if e.Inference.ScoreGap > 0 {
				t.Status, t.Reason, t.SuggestedRate = "estimated", "product_preference", &rate
			}
		}
	}
	return e
}

func validTaxRate(rate *int64) bool { return rate != nil && *rate > 0 && *rate <= 100 }

func rejectTaxInference(e AmountEvidence, reason string) AmountEvidence {
	e.Inference.Status, e.Inference.Reason = "conflict", reason
	for i := range e.DetailTaxes {
		e.DetailTaxes[i].Rate, e.DetailTaxes[i].SuggestedRate = nil, nil
		e.DetailTaxes[i].Mode = "unknown"
		e.DetailTaxes[i].Status, e.DetailTaxes[i].Reason = "unresolved", reason
	}
	return e
}

func inferenceGroups(taxes []TaxBreakdown) ([]taxGroup, string) {
	if len(taxes) == 0 || len(taxes) > 2 {
		return nil, "insufficient_evidence"
	}
	groups := make([]taxGroup, 0, len(taxes))
	for _, t := range taxes {
		if t.Rate == nil || (*t.Rate != 8 && *t.Rate != 10) || (t.Mode != "external" && t.Mode != "included") {
			return nil, "unsupported_tax_group"
		}
		for _, g := range groups {
			if g.rate == *t.Rate {
				return nil, "inconsistent_breakdown"
			}
			if g.mode != t.Mode {
				return nil, "mixed_modes"
			}
		}
		if t.TaxAmount == nil {
			return nil, "insufficient_evidence"
		}
		if *t.TaxAmount < 0 || *t.TaxAmount > 10_000_000 {
			return nil, "inconsistent_breakdown"
		}
		var base, gross int64
		if t.TaxableAmount != nil {
			base = *t.TaxableAmount
			gross = base + *t.TaxAmount
			if t.GrossAmount != nil && *t.GrossAmount != gross {
				return nil, "inconsistent_breakdown"
			}
		} else if t.Mode == "included" && t.GrossAmount != nil {
			gross = *t.GrossAmount
			base = gross - *t.TaxAmount
		} else {
			return nil, "insufficient_evidence"
		}
		if base < 0 || gross < 0 || gross > 10_000_000 {
			return nil, "inconsistent_breakdown"
		}
		// 印字税額を優先するが、税率と明らかに矛盾する内訳は根拠にしない。
		numerator, denominator := base*(*t.Rate), int64(100)
		if t.Mode == "included" {
			numerator, denominator = gross*(*t.Rate), 100+*t.Rate
		}
		if *t.TaxAmount < numerator/denominator || *t.TaxAmount > (numerator+denominator-1)/denominator {
			return nil, "inconsistent_breakdown"
		}
		target := base
		if t.Mode == "included" {
			target = gross
		}
		groups = append(groups, taxGroup{rate: *t.Rate, target: target, mode: t.Mode})
	}
	return groups, ""
}

// 同じ部分和に至る経路は上位2件へ集約する。将来の選択肢は部分和だけで決まるため、
// 一意性と最良・次点の差を保ったまま指数的な経路の保持を避けられる。
func searchTaxAssignments(details []ReadDetail, amounts []int64, taxes []DetailTax, groups []taxGroup) ([]taxAssignment, int, bool) {
	states := map[int64][]taxAssignment{0: {{}}}
	remaining := int64(0)
	for _, a := range amounts {
		remaining += a
	}
	visited := 0
	for i, a := range amounts {
		remaining -= a
		next := map[int64][]taxAssignment{}
		for sum, paths := range states {
			for g, group := range groups {
				if taxes[i].Rate != nil && *taxes[i].Rate != group.rate {
					continue
				}
				nextSum := sum
				if g == 0 {
					nextSum += a
				}
				if nextSum > groups[0].target || nextSum+remaining < groups[0].target {
					continue
				}
				for _, path := range paths {
					visited++
					if visited > maxTaxSearchTransitions {
						return nil, visited, true
					}
					candidate := path
					if g == 1 {
						candidate.mask |= uint64(1) << i
					}
					candidate.score += taxPreference(details[i], group.rate)
					next[nextSum] = bestTwoAssignments(next[nextSum], candidate)
				}
			}
		}
		states = next
	}
	return states[groups[0].target], visited, false
}

func bestTwoAssignments(paths []taxAssignment, candidate taxAssignment) []taxAssignment {
	paths = append(paths, candidate)
	for i := len(paths) - 1; i > 0; i-- {
		if paths[i].score < paths[i-1].score || (paths[i].score == paths[i-1].score && paths[i].mask > paths[i-1].mask) {
			break
		}
		paths[i], paths[i-1] = paths[i-1], paths[i]
	}
	if len(paths) > 2 {
		paths = paths[:2]
	}
	return paths
}

func taxPreference(d ReadDetail, rate int64) int {
	preferred := int64(0)
	switch d.Category {
	case "food":
		preferred = 8
	case "daily_goods", "clothing":
		preferred = 10
	}
	score := 0
	if rate == preferred {
		score++
	}
	// 品名は順位付けだけに使用する。曖昧な名称やカテゴリの誤分類があっても確定しない。
	for _, word := range []string{"パン", "牛乳", "おにぎり", "弁当", "カレー"} {
		if strings.Contains(d.Name, word) && rate == 8 {
			score += 2
			break
		}
	}
	for _, word := range []string{"洗剤", "タオル", "クリーナー", "レジ袋", "ビール", "ワイン"} {
		if strings.Contains(d.Name, word) && rate == 10 {
			score += 2
			break
		}
	}
	return score
}

// resolveTaxModes は商品合計・税率別対象額・支払合計の一致から不明な課税方式を補う。
// 内税・外税の両方が成立する（税額0円など）場合は補完しない。
func resolveTaxModes(r ReceiptReading, selected int64) []TaxBreakdown {
	taxes := append([]TaxBreakdown(nil), r.TaxBreakdown...)
	var printed int64
	for _, d := range r.Details {
		if d.Amount < 0 || d.Amount > 10_000_000 {
			return taxes
		}
		printed += d.Amount
	}
	printed -= receiptDiscount(r.AmountCandidates)
	matches := []string{}
	for _, mode := range []string{"included", "external"} {
		var target, gross int64
		valid := len(taxes) > 0
		for _, tax := range taxes {
			if tax.Mode != "" && tax.Mode != "unknown" && tax.Mode != mode {
				valid = false
				break
			}
			if tax.TaxAmount == nil || *tax.TaxAmount < 0 || *tax.TaxAmount > 10_000_000 {
				valid = false
				break
			}
			var amount int64
			if mode == "external" {
				if tax.TaxableAmount == nil {
					valid = false
					break
				}
				amount = *tax.TaxableAmount
			} else if tax.GrossAmount != nil {
				amount = *tax.GrossAmount
			} else if tax.TaxableAmount != nil && *tax.TaxableAmount >= 0 && *tax.TaxableAmount <= 10_000_000 {
				amount = *tax.TaxableAmount + *tax.TaxAmount
			} else {
				valid = false
				break
			}
			if amount < 0 || amount > 10_000_000 {
				valid = false
				break
			}
			target += amount
			gross += amount
			if mode == "external" {
				gross += *tax.TaxAmount
			}
		}
		if valid && target == printed && gross == selected {
			matches = append(matches, mode)
		}
	}
	if len(matches) == 1 {
		for i := range taxes {
			if taxes[i].Mode == "unknown" || taxes[i].Mode == "" {
				taxes[i].OriginalMode, taxes[i].Mode = taxes[i].Mode, matches[0]
			}
		}
	}
	return taxes
}
