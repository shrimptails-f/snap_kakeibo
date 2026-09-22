import { formatYen } from '@/shared/lib/formatYen'
import { useExpense } from '../hooks/useExpense'
import { categoryLabel } from '../lib/categoryLabel'
import { sourceLabel } from '../lib/sourceLabel'
import styles from './ExpenseDetailPanel.module.css'

type Props = {
  expenseId: string
}

// 支出の要約と明細。呼び出し側が Suspense で包み、取得中の表示を決める
export function ExpenseDetailPanel({ expenseId }: Props) {
  const { data } = useExpense(expenseId)
  const { expense, details } = data

  return (
    <>
      <div className={styles.summary}>
        <span>{expense.store_name}</span>
        <strong className="amount">{formatYen(expense.recorded_amount)}</strong>
        <small>{expense.purchase_date}</small>
        <small>
          {sourceLabel(expense.source)}
          {expense.is_edited && '・手動編集済み'} / 最終更新: {new Date(expense.updated_at).toLocaleString('ja-JP')}
        </small>
        {expense.adjustment_amount !== 0 && (
          <small>
            読取金額 {formatYen(expense.read_amount)} / 調整額 {formatYen(expense.adjustment_amount)}
          </small>
        )}
      </div>
      <table className={styles.details}>
        <thead>
          <tr>
            <th>品目</th>
            <th>カテゴリ</th>
            <th>金額</th>
          </tr>
        </thead>
        <tbody>
          {details.map((detail) => (
            <tr key={detail.detail_id}>
              <td>
                {detail.name}
                {detail.is_edited && <small>（手動編集済み）</small>}
              </td>
              <td>{categoryLabel(detail.category)}</td>
              <td className="amount">{formatYen(detail.amount)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  )
}
