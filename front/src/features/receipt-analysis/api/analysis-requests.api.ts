import { http } from '@/shared/api/http'
import { parseResponse } from '@/shared/api/parseResponse'
import { listAnalysisRequestsResponseSchema } from '../types/analysis-request.schema'
import type { ListAnalysisRequestsResponse } from '../types/analysis-request.types'


export async function listAnalysisRequests(yearMonth: string, signal?: AbortSignal): Promise<ListAnalysisRequestsResponse> {
  const raw = await http.get<unknown>(`/api/months/${encodeURIComponent(yearMonth)}/analysis-requests`, { signal })
  return parseResponse(listAnalysisRequestsResponseSchema, raw, 'GET /api/months/{yyyy-MM}/analysis-requests')
}
