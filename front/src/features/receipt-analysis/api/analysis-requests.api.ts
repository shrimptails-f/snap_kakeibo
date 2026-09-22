import { http } from '@/shared/api/http'
import { parseResponse } from '@/shared/api/parseResponse'
import { listAnalysisRequestsResponseSchema } from '../types/analysis-request.schema'
import type { AnalysisRequestFilter, ListAnalysisRequestsResponse } from '../types/analysis-request.types'

export async function listAnalysisRequests(yearMonth: string, filter: AnalysisRequestFilter, cursor: string, signal?: AbortSignal): Promise<ListAnalysisRequestsResponse> {
  const query = new URLSearchParams({ filter })
  if (cursor) query.set('cursor', cursor)
  const raw = await http.get<unknown>(`/api/months/${encodeURIComponent(yearMonth)}/analysis-requests?${query}`, { signal })
  return parseResponse(listAnalysisRequestsResponseSchema, raw, 'GET /api/months/{yyyy-MM}/analysis-requests')
}
