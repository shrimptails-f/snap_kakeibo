import { useMutation } from '@tanstack/react-query'
import { updateExpense } from '../api/expenses.api'

export function useUpdateExpense(expenseId: string) {
  return useMutation({ mutationFn: (body: Parameters<typeof updateExpense>[1]) => updateExpense(expenseId, body) })
}
