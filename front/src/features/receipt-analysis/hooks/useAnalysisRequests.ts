import { useSuspenseQuery } from '@tanstack/react-query'
import { listAnalysisRequests } from '../api/analysis-requests.api'

export const analysisRequestsQueryKey = (yearMonth: string) => ['analysis-requests', yearMonth] as const

// 指定月の解析依頼一覧。初回は Suspense で待ち、再取得中は前回の内容を保つ。
// 解析の進み具合は定期取得せず、利用者が再読み込みボタン(useReloadAnalysisRequests)で確かめる
export function useAnalysisRequests(yearMonth: string) {
  return useSuspenseQuery({
    queryKey: analysisRequestsQueryKey(yearMonth),
    queryFn: ({ signal }) => listAnalysisRequests(yearMonth, signal),
  })
}
