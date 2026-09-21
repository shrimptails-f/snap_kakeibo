import { http } from '@/shared/api/http'
import type { Expense, ExpenseDetail } from '../types/expense.types'

export type GetExpenseResponse = {
  expense: Expense
  details: ExpenseDetail[]
}

function isGetExpenseResponse(value: unknown): value is GetExpenseResponse {
  if (typeof value !== 'object' || value === null) return false
  const candidate = value as Record<string, unknown>
  return typeof candidate.expense === 'object' && candidate.expense !== null && Array.isArray(candidate.details)
}

export async function getExpense(expenseId: string, signal?: AbortSignal): Promise<GetExpenseResponse> {
  const response = await http.get<unknown>(`/api/expenses/${encodeURIComponent(expenseId)}`, { signal })
  if (!isGetExpenseResponse(response)) {
    throw new Error('unexpected response shape: GET /api/expenses/{expense_id}')
  }
  return response
}
