import { useSuspenseQuery } from '@tanstack/react-query'
import { listAnalysisRequests } from '../api/analysis-requests.api'
import type { AnalysisRequestFilter } from '../types/analysis-request.types'

export const analysisRequestsQueryPrefix = (yearMonth: string) => ['analysis-requests', yearMonth] as const
export const analysisRequestsQueryKey = (yearMonth: string, filter: AnalysisRequestFilter, cursor: string) => [...analysisRequestsQueryPrefix(yearMonth), filter, cursor] as const

// 指定月の解析依頼一覧。初回は Suspense で待ち、再取得中は前回の内容を保つ。
// 解析の進み具合は定期取得せず、利用者が再読み込みボタン(useReloadAnalysisRequests)で確かめる
export function useAnalysisRequests(yearMonth: string, filter: AnalysisRequestFilter, cursor: string) {
  return useSuspenseQuery({
    queryKey: analysisRequestsQueryKey(yearMonth, filter, cursor),
    queryFn: ({ signal }) => listAnalysisRequests(yearMonth, filter, cursor, signal),
  })
}
