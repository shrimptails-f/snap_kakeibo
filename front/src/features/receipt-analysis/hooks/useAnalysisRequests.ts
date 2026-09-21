import { useSuspenseQuery } from '@tanstack/react-query'
import { listAnalysisRequests } from '../api/analysis-requests.api'

// 解析依頼は非同期に状態が進むので、画面を開いている間は定期的に再取得する
const REFETCH_INTERVAL_MS = 5000

export const analysisRequestsQueryKey = (yearMonth: string) => ['analysis-requests', yearMonth] as const

// 指定月の解析依頼一覧。初回は Suspense で待ち、再取得中は前回の内容を保つ
export function useAnalysisRequests(yearMonth: string) {
  return useSuspenseQuery({
    queryKey: analysisRequestsQueryKey(yearMonth),
    queryFn: ({ signal }) => listAnalysisRequests(yearMonth, signal),
    refetchInterval: REFETCH_INTERVAL_MS,
  })
}
