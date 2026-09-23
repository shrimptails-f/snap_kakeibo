package domain

import "strings"

// AllocateDetailTaxes は印字された税額だけを商品行へ配分する。
// 課税対象額と商品行を照合できない税率群は未確定のまま残す。
func AllocateDetailTaxes(details []ReadDetail, evidence AmountEvidence) []*int64 {
	result := make([]*int64, len(details))
	if len(details) != len(evidence.DetailTaxes) {
		return result
	}
	groups := map[int64][]int{}
	for i, detail := range details {
		tax := evidence.DetailTaxes[i]
		if tax.Mode == "included" {
			amount := detail.Amount
			result[i] = &amount
		} else if tax.Mode == "external" && tax.Rate != nil && *tax.Rate > 0 && *tax.Rate <= 100 {
			groups[*tax.Rate] = append(groups[*tax.Rate], i)
		}
	}
	if len(groups) == 0 {
		return result
	}
	breakdowns := map[int64]TaxBreakdown{}
	duplicates := map[int64]bool{}
	for _, tax := range evidence.Taxes {
		if tax.Mode != "external" || tax.Rate == nil || tax.TaxableAmount == nil || tax.TaxAmount == nil || *tax.TaxAmount < 0 || *tax.TaxAmount > 10_000_000 || *tax.TaxableAmount < 0 {
			continue
		}
		if duplicates[*tax.Rate] {
			continue
		}
		if _, exists := breakdowns[*tax.Rate]; exists {
			delete(breakdowns, *tax.Rate)
			duplicates[*tax.Rate] = true
			continue
		}
		breakdowns[*tax.Rate] = tax
	}
	var printed, taxable int64
	missingBreakdown := false
	for rate, indices := range groups {
		tax, ok := breakdowns[rate]
		if !ok {
			missingBreakdown = true
			continue
		}
		taxable += *tax.TaxableAmount
		var groupPrinted int64
		for _, index := range indices {
			groupPrinted += details[index].Amount
		}
		if groupPrinted < *tax.TaxableAmount {
			return result
		}
		printed += groupPrinted
	}
	// 税率別の課税対象額との差が、レシートの値引合計と一致するときだけ換算する。
	if printed != taxable && (missingBreakdown || printed-taxable != receiptDiscount(evidence.Candidates)) {
		return result
	}
	for rate, indices := range groups {
		tax, ok := breakdowns[rate]
		if !ok {
			continue
		}
		var sum int64
		for _, index := range indices {
			sum += details[index].Amount
		}
		if sum == 0 || sum < *tax.TaxableAmount {
			continue
		}
		// 最大剰余法。同じ剰余なら印字順を優先し、配分後の合計を印字税額に一致させる。
		allocated := make([]int64, len(indices))
		var used int64
		for j, index := range indices {
			allocated[j] = *tax.TaxAmount * details[index].Amount / sum
			used += allocated[j]
		}
		for ; used < *tax.TaxAmount; used++ {
			best := -1
			var remainder int64 = -1
			for j, index := range indices {
				left := (*tax.TaxAmount*details[index].Amount)%sum - (allocated[j]-*tax.TaxAmount*details[index].Amount/sum)*sum
				if left > remainder {
					best, remainder = j, left
				}
			}
			allocated[best]++
		}
		valid := true
		for j, index := range indices {
			amount := details[index].Amount + allocated[j]
			if amount > 10_000_000 {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}
		for j, index := range indices {
			amount := details[index].Amount + allocated[j]
			result[index] = &amount
		}
	}
	return result
}

func receiptDiscount(candidates []AmountCandidate) int64 {
	var total, summary int64
	hasSummary := false
	for _, candidate := range candidates {
		if candidateRole(candidate) != "discount" {
			continue
		}
		value := absAmount(candidate.Amount)
		if strings.Contains(candidate.Label, "値引合計") || strings.Contains(candidate.Label, "割引合計") {
			summary = value
			hasSummary = true
		} else {
			total += value
		}
	}
	if hasSummary {
		return summary
	}
	return total
}
