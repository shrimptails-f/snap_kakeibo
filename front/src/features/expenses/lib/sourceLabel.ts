import type { ExpenseSource } from '../types/expense.types'

export function sourceLabel(source: ExpenseSource): string {
  return source === 'AI' ? 'AI由来' : 'ユーザー入力'
}
