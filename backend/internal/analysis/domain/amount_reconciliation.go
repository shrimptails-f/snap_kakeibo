package domain

import "strings"

// AmountCandidate は印字された金額行。Position は上からの順番で、大きいほど下部にある。
type AmountCandidate struct {
	Amount   int64  `json:"amount"`
	Label    string `json:"label"`
	Role     string `json:"role"`
	Position int64  `json:"position"`
}

// TaxBreakdown は税率ごとの印字された税抜対象額と税額。nil は判読不能を示す。
type TaxBreakdown struct {
	Rate          *int64 `json:"rate"`
	TaxableAmount *int64 `json:"taxable_amount"`
	TaxAmount     *int64 `json:"tax_amount"`
	Mode          string `json:"mode"`
}

// AmountEvidence は支払合計の候補、税区分、照合状態と根拠を保存する。
type AmountEvidence struct {
	Selected    *AmountCandidate  `json:"selected"`
	Candidates  []AmountCandidate `json:"candidates"`
	Taxes       []TaxBreakdown    `json:"taxes"`
	DetailTaxes []DetailTax       `json:"detail_taxes"`
	TaxMode     string            `json:"tax_mode"`
	Status      string            `json:"status"`
	Reasons     []string          `json:"reasons"`
}

// DetailTax は商品行ごとの読取税率と内外税区分。
type DetailTax struct {
	Rate *int64 `json:"rate"`
	Mode string `json:"mode"`
}

// ReconcileAmounts は印字上の役割と金額関係から最終支払額を選ぶ。
// 明細の読取漏れを許し、金額を補正したり差額を税として推測したりしない。
func ReconcileAmounts(r ReceiptReading) AmountEvidence {
	e := AmountEvidence{Candidates: append([]AmountCandidate(nil), r.AmountCandidates...), Taxes: append([]TaxBreakdown(nil), r.TaxBreakdown...)}
	for _, d := range r.Details {
		e.DetailTaxes = append(e.DetailTaxes, DetailTax{Rate: d.TaxRate, Mode: d.TaxMode})
	}
	e.TaxMode = taxMode(r)
	if len(r.AmountCandidates) == 0 {
		if r.ReadAmount != nil {
			e.Selected = &AmountCandidate{Amount: *r.ReadAmount, Role: "final_total"}
			e.Status = "weak"
			e.Reasons = []string{"legacy_total_without_candidates"}
		}
		return e
	}
	var discount, externalTax int64
	var subtotals []int64
	var discountSummary *int64
	paymentCount := 0
	var estimatedTax, estimatedGroups int64
	for _, c := range r.AmountCandidates {
		switch candidateRole(c) {
		case "payment":
			paymentCount++
		case "subtotal":
			subtotals = append(subtotals, c.Amount)
		case "discount":
			value := absAmount(c.Amount)
			if strings.Contains(c.Label, "値引合計") || strings.Contains(c.Label, "割引合計") {
				discountSummary = &value
			} else {
				discount += value
			}
		}
	}
	if discountSummary != nil {
		discount = *discountSummary
	}
	for _, tax := range r.TaxBreakdown {
		if tax.Mode == "external" && tax.TaxAmount != nil {
			externalTax += *tax.TaxAmount
		} else if tax.Mode == "external" && tax.TaxAmount == nil && tax.TaxableAmount != nil && tax.Rate != nil && *tax.Rate > 0 && *tax.Rate <= 100 {
			estimatedTax += *tax.TaxableAmount * *tax.Rate / 100
			estimatedGroups++
		}
	}
	breakdownGross, completeBreakdown := int64(0), len(r.TaxBreakdown) > 0
	for _, tax := range r.TaxBreakdown {
		if tax.TaxableAmount == nil || tax.TaxAmount == nil {
			completeBreakdown = false
			break
		}
		breakdownGross += *tax.TaxableAmount + *tax.TaxAmount
	}
	best, second := -1<<30, -1<<30
	for i, c := range r.AmountCandidates {
		if c.Amount < 0 || c.Amount > 10_000_000 {
			continue
		}
		role := candidateRole(c)
		if role == "tax" || role == "taxable" || role == "deposit" || role == "change" || role == "discount" || role == "subtotal" || role == "payment" && paymentCount > 1 {
			continue
		}
		score := 0
		if role == "final_total" {
			score = 100
		} else if role == "payment" {
			score = 40
		}
		if c.Position > 0 {
			score += int(min(c.Position, 20))
		}
		for _, subtotal := range subtotals {
			if estimatedGroups == 0 && c.Amount == subtotal-discount+externalTax {
				score += 30
				break
			}
			if estimatedGroups > 0 && absAmount(c.Amount-(subtotal-discount+externalTax+estimatedTax)) <= estimatedGroups {
				score += 10
				break
			}
		}
		if completeBreakdown && c.Amount == breakdownGross {
			score += 40
		}
		var details int64
		for _, d := range r.Details {
			details += d.Amount
		}
		if len(r.Details) > 0 && (c.Amount == details || c.Amount == details-discount+externalTax) {
			score += 10
		}
		if role == "final_total" {
			for _, other := range r.AmountCandidates {
				if candidateRole(other) == "payment" && other.Amount == c.Amount {
					score += 15
					break
				}
			}
		}
		for _, tax := range r.TaxBreakdown {
			if tax.TaxAmount != nil && c.Amount == *tax.TaxAmount {
				score -= 30
			}
			if tax.TaxableAmount != nil && c.Amount == *tax.TaxableAmount {
				score -= 20
			}
		}
		if score > best {
			second = best
			best = score
			selected := r.AmountCandidates[i]
			e.Selected = &selected
		} else if score > second {
			second = score
		}
	}
	if e.Selected == nil {
		return e
	}
	e.Status = "weak"
	if candidateRole(*e.Selected) == "final_total" && best-second >= 20 {
		e.Status = "strong"
	}
	for _, subtotal := range subtotals {
		if estimatedGroups == 0 && e.Selected.Amount == subtotal-discount+externalTax {
			e.Reasons = append(e.Reasons, "subtotal_discount_tax_match")
			break
		}
	}
	if completeBreakdown && e.Selected.Amount == breakdownGross {
		e.Reasons = append(e.Reasons, "tax_breakdown_match")
	}
	for _, other := range r.AmountCandidates {
		if candidateRole(other) == "payment" && other.Amount == e.Selected.Amount {
			e.Reasons = append(e.Reasons, "payment_match")
			break
		}
	}
	if candidateRole(*e.Selected) == "final_total" {
		e.Reasons = append(e.Reasons, "final_total_label")
	}
	if e.Status == "weak" {
		e.Reasons = append(e.Reasons, "ambiguous_or_incomplete")
	}
	return e
}

func absAmount(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func taxMode(r ReceiptReading) string {
	included, external, unknown := false, false, false
	add := func(mode string) {
		switch mode {
		case "included":
			included = true
		case "external":
			external = true
		case "mixed":
			included, external = true, true
		default:
			unknown = true
		}
	}
	for _, tax := range r.TaxBreakdown {
		add(tax.Mode)
	}
	if len(r.TaxBreakdown) == 0 {
		for _, detail := range r.Details {
			add(detail.TaxMode)
		}
	}
	if included && external {
		return "mixed"
	}
	if unknown || (!included && !external) {
		return "unknown"
	}
	if included {
		return "included"
	}
	return "external"
}

func candidateRole(c AmountCandidate) string {
	label := strings.ToLower(strings.TrimSpace(c.Label))
	// ラベルが役割と矛盾する場合は印字されたラベルを優先する。
	switch {
	case strings.Contains(label, "預り"), strings.Contains(label, "お預かり"), strings.Contains(label, "お預り"), strings.Contains(label, "cash tendered"):
		return "deposit"
	case strings.Contains(label, "釣"), strings.Contains(label, "change"):
		return "change"
	case strings.Contains(label, "値引"), strings.Contains(label, "割引"), strings.Contains(label, "クーポン"), strings.Contains(label, "discount"):
		return "discount"
	case strings.Contains(label, "税額"), strings.Contains(label, "消費税"), strings.Contains(label, "内税"), strings.Contains(label, "外税"), strings.Contains(label, "tax amount"):
		return "tax"
	case strings.Contains(label, "税抜対象"), strings.Contains(label, "対象額"), strings.Contains(label, "課税対象"), strings.Contains(label, "taxable"):
		return "taxable"
	case (strings.Contains(label, "税率") || strings.Contains(label, "%")) && strings.Contains(label, "対象"):
		return "taxable"
	case strings.Contains(label, "小計"), strings.Contains(label, "商品代金"), strings.Contains(label, "subtotal"):
		return "subtotal"
	case strings.Contains(label, "合計"), strings.Contains(label, "お買上"), strings.Contains(label, "お支払"), strings.Contains(label, "請求額"), strings.Contains(label, "total"):
		return "final_total"
	case strings.Contains(label, "支払"), strings.Contains(label, "決済"):
		return "payment"
	}
	return c.Role
}
