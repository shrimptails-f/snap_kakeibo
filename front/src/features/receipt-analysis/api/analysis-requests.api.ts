import { http } from '@/shared/api/http'
import type { AnalysisRequestItem } from '../types/analysis-request.types'

export type ListAnalysisRequestsResponse = {
  items: AnalysisRequestItem[]
}

function isListAnalysisRequestsResponse(value: unknown): value is ListAnalysisRequestsResponse {
  return typeof value === 'object' && value !== null && Array.isArray((value as Record<string, unknown>).items)
}

export async function listAnalysisRequests(yearMonth: string, signal?: AbortSignal): Promise<ListAnalysisRequestsResponse> {
  const response = await http.get<unknown>(`/api/months/${encodeURIComponent(yearMonth)}/analysis-requests`, { signal })
  if (!isListAnalysisRequestsResponse(response)) {
    throw new Error('unexpected response shape: GET /api/months/{yyyy-MM}/analysis-requests')
  }
  return response
}
