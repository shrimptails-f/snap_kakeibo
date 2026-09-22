import { http } from '@/shared/api/http'
import { parseResponse } from '@/shared/api/parseResponse'
import { retryAnalysisResponseSchema } from '../types/analysis-request.schema'
import type { RetryAnalysisResponse } from '../types/analysis-request.types'

export async function retryAnalysis(analysisRequestId: string): Promise<RetryAnalysisResponse> {
  const raw = await http.post<unknown, undefined>(
    `/api/analysis-requests/${encodeURIComponent(analysisRequestId)}/retry`,
  )
  return parseResponse(retryAnalysisResponseSchema, raw, 'POST /api/analysis-requests/{analysisRequestId}/retry')
}
