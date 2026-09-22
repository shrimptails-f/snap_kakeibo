import { http } from '@/shared/api/http'
import { parseResponse } from '@/shared/api/parseResponse'
import { getMonthlySummariesResponseSchema } from '../types/monthly-summary.schema'
import type { GetMonthlySummariesResponse } from '../types/monthly-summary.types'

export async function getMonthlySummaries(signal?: AbortSignal): Promise<GetMonthlySummariesResponse> {
  const raw = await http.get<unknown>('/api/monthly-summaries', { signal })
  return parseResponse(getMonthlySummariesResponseSchema, raw, 'GET /api/monthly-summaries')
}
