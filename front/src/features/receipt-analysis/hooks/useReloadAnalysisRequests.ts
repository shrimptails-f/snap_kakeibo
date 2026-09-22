import { useEffect, useState } from 'react'
import { useIsFetching, useQueryClient } from '@tanstack/react-query'
import { analysisRequestsQueryKey } from './useAnalysisRequests'

// 連打で API を叩かないよう、再読み込みのあと 5 秒はボタンを無効にする
export const RELOAD_COOLDOWN_MS = 5000

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
export function useReloadAnalysisRequests(yearMonth: string): ReloadAnalysisRequests {
  const queryClient = useQueryClient()
  const queryKey = analysisRequestsQueryKey(yearMonth)
  const isFetching = useIsFetching({ queryKey }) > 0
  const [cooldownUntil, setCooldownUntil] = useState<number | null>(null)
  const [cooldownRemainingMs, setCooldownRemainingMs] = useState(0)
  const [hasError, setHasError] = useState(false)

  // 残り時間を表示しつつ、クールタイムが明けたらボタンを戻す
  useEffect(() => {
    if (cooldownUntil === null) return
    const deadline = cooldownUntil
    function updateRemaining() {
      const remaining = Math.max(0, deadline - Date.now())
      setCooldownRemainingMs(remaining)
      if (remaining === 0) setCooldownUntil(null)
    }
    updateRemaining()
    const timer = setInterval(updateRemaining, 100)
    return () => clearInterval(timer)
  }, [cooldownUntil])

  function reload() {
    if (cooldownUntil !== null) return
    const nextCooldownUntil = Date.now() + RELOAD_COOLDOWN_MS
    setCooldownRemainingMs(RELOAD_COOLDOWN_MS)
    setCooldownUntil(nextCooldownUntil)
    setHasError(false)
    // 失敗しても前回の一覧を保ち、取得結果だけを案内する
    void queryClient.refetchQueries({ queryKey }).then(() => {
      setHasError(queryClient.getQueryState(queryKey)?.error != null)
    })
  }

  return {
    reload,
    isDisabled: isFetching || cooldownUntil !== null,
    isFetching,
    isCoolingDown: cooldownUntil !== null,
    cooldownRemainingMs,
    hasError,
  }
}
