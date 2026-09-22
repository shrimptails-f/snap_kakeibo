import { useSuspenseQuery } from '@tanstack/react-query'
import { getExpense } from '../api/expenses.api'

export const expenseQueryKey = (expenseId: string) => ['expenses', expenseId] as const

// 支出と明細。初回は Suspense で待つ
export function useExpense(expenseId: string) {
  return useSuspenseQuery({
    queryKey: expenseQueryKey(expenseId),
    queryFn: ({ signal }) => getExpense(expenseId, signal),
  })
}
