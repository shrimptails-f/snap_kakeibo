import { useMutation, useQueryClient } from '@tanstack/react-query'
import { retryAnalysis } from '../api/retry-analysis.api'
import { analysisRequestsQueryPrefix } from './useAnalysisRequests'

export function useRetryAnalysis(yearMonth: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: retryAnalysis,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: analysisRequestsQueryPrefix(yearMonth) }),
  })
}
