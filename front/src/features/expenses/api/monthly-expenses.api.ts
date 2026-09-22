import { http } from '@/shared/api/http'
import { parseResponse } from '@/shared/api/parseResponse'
import { getMonthlySummariesResponseSchema, listMonthExpensesResponseSchema } from '../types/monthly-expenses.schema'
import type { GetMonthlySummariesResponse, ListMonthExpensesResponse } from '../types/monthly-expenses.types'

export async function getMonthlySummaries(signal?: AbortSignal): Promise<GetMonthlySummariesResponse> {
  const raw = await http.get<unknown>('/api/monthly-summaries', { signal })
  return parseResponse(getMonthlySummariesResponseSchema, raw, 'GET /api/monthly-summaries')
}

export async function listMonthExpenses(yearMonth: string, signal?: AbortSignal): Promise<ListMonthExpensesResponse> {
  const raw = await http.get<unknown>(`/api/months/${encodeURIComponent(yearMonth)}/expenses`, { signal })
  const response = parseResponse(listMonthExpensesResponseSchema, raw, 'GET /api/months/{yyyy-MM}/expenses')
  if (response.year_month !== yearMonth) throw new Error('unexpected response month')
  return response
}
