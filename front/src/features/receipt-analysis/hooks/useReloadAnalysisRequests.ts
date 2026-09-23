import { useState } from 'react'
import { useIsFetching, useQueryClient } from '@tanstack/react-query'
import { useReloadCooldown } from '@/shared/hooks/useReloadCooldown'
import { analysisRequestsQueryKey } from './useAnalysisRequests'
import type { AnalysisRequestFilter } from '../types/analysis-request.types'

export type ReloadAnalysisRequests = {
  reload: () => void
  // 取得中またはクールタイム中
  isDisabled: boolean
  isFetching: boolean
  isCoolingDown: boolean
  cooldownRemainingMs: number
  hasError: boolean
}

// 解析依頼一覧の再読み込み。再取得中は useAnalysisRequests が前回の一覧を保つので、画面が消えることはない
export function useReloadAnalysisRequests(yearMonth: string, filter: AnalysisRequestFilter, cursor: string): ReloadAnalysisRequests {
  const queryClient = useQueryClient()
  const queryKey = analysisRequestsQueryKey(yearMonth, filter, cursor)
  const isFetching = useIsFetching({ queryKey }) > 0
  const { startCooldown, isCoolingDown, cooldownRemainingMs } = useReloadCooldown()
  const [hasError, setHasError] = useState(false)

  function reload() {
    if (!startCooldown()) return
    setHasError(false)
    // 失敗しても前回の一覧を保ち、取得結果だけを案内する
    void queryClient.refetchQueries({ queryKey }).then(() => {
      setHasError(queryClient.getQueryState(queryKey)?.error != null)
    })
  }

  return {
    reload,
    isDisabled: isFetching || isCoolingDown,
    isFetching,
    isCoolingDown,
    cooldownRemainingMs,
    hasError,
  }
}
