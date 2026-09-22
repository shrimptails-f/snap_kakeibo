import { useQuery } from '@tanstack/react-query'
import { getMonthlySummaries } from '../api/monthly-summaries.api'

// 月別支出と同じ API の cache を共有し、支出編集後の invalidate をダッシュボードにも反映する。
export const dashboardMonthlySummariesQueryKey = ['monthly-summaries'] as const

export function useMonthlySummaries() {
  return useQuery({
    queryKey: dashboardMonthlySummariesQueryKey,
    queryFn: ({ signal }) => getMonthlySummaries(signal),
  })
}
