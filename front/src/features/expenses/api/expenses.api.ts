import { http } from '@/shared/api/http'
import { parseResponse } from '@/shared/api/parseResponse'
import { getExpenseResponseSchema, updateExpenseResponseSchema } from '../types/expense.schema'
import type { GetExpenseResponse, UpdateExpenseRequest, UpdateExpenseResponse } from '../types/expense.types'


export async function getExpense(expenseId: string, signal?: AbortSignal): Promise<GetExpenseResponse> {
  const raw = await http.get<unknown>(`/api/expenses/${encodeURIComponent(expenseId)}`, { signal })
  return parseResponse(getExpenseResponseSchema, raw, 'GET /api/expenses/{expense_id}')
}

export async function updateExpense(expenseId: string, body: UpdateExpenseRequest): Promise<UpdateExpenseResponse> {
  const raw = await http.patch<unknown, UpdateExpenseRequest>(`/api/expenses/${encodeURIComponent(expenseId)}`, { body })
  return parseResponse(updateExpenseResponseSchema, raw, 'PATCH /api/expenses/{expense_id}')
}
