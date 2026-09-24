import type { ExpenseDetail } from '../types/expense.types'

type Props = { detail: ExpenseDetail }

const reasons: Record<string, string> = {
  conflicting_marks: '商品の印と税率が一致しません。',
  inconsistent_breakdown: '税率別の対象額と税額が一致しません。',
  amount_mismatch: '商品・値引き・税率別合計を照合できませんでした。',
  inconsistent_discount: '値引きの内訳を照合できませんでした。',
  invalid_discount: '商品の値引額を確認できませんでした。',
  invalid_detail: '商品行を確認できませんでした。',
  conflicting_mode: '内税・外税の記載が一致しません。',
  conflicting_rate: '商品と合計欄の税率が一致しません。',
  no_solution: '税率別合計に一致する組み合わせがありません。',
  multiple_solutions: '税率別合計に一致する組み合わせが複数あります。',
  search_limit: '組み合わせが多いため税率を確定できませんでした。',
  unassigned_discount: '値引きの対象商品が不明です。',
  product_changed: '商品情報が変更されたため推定を解除しました。',
  amount_changed: '印字額が変更されたため税情報の再確認が必要です。',
}

export function TaxEvidenceNote({ detail }: Props) {
  switch (detail.tax_status) {
    case 'printed':
      return <span>{detail.tax_reason === 'printed_mark' ? '税率の根拠：商品の印とレシートの注記' : '税率の根拠：レシートの記載'}</span>
    case 'reconciled':
      return <span>税率別合計と一致する組み合わせから税率を補完しました。</span>
    case 'estimated':
      return <span>推定税率 {detail.suggested_tax_rate}%（未確定）。合計に一致する候補を商品名・カテゴリで順位付けしています。レシートを確認し、編集画面で税区分・税率を指定してください。</span>
    case 'unresolved':
      return <span>{reasons[detail.tax_reason ?? ''] ?? '税率を補完する根拠が不足しています。'}</span>
    case 'user_confirmed':
      return <span>税情報は利用者が確認済みです。</span>
    default:
      return null
  }
}
