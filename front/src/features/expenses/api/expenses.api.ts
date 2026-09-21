import { http } from '@/shared/api/http'
import { parseResponse } from '@/shared/api/parseResponse'
import { getExpenseResponseSchema } from '../types/expense.schema'
import type { GetExpenseResponse } from '../types/expense.types'


export async function getExpense(expenseId: string, signal?: AbortSignal): Promise<GetExpenseResponse> {
  const raw = await http.get<unknown>(`/api/expenses/${encodeURIComponent(expenseId)}`, { signal })
  return parseResponse(getExpenseResponseSchema, raw, 'GET /api/expenses/{expense_id}')
}
