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
}

// 解析依頼一覧の再読み込み。再取得中は useAnalysisRequests が前回の一覧を保つので、画面が消えることはない
export function useReloadAnalysisRequests(yearMonth: string): ReloadAnalysisRequests {
  const queryClient = useQueryClient()
  const queryKey = analysisRequestsQueryKey(yearMonth)
  const isFetching = useIsFetching({ queryKey }) > 0
  const [cooldownUntil, setCooldownUntil] = useState<number | null>(null)

  // クールタイムが明けたら再描画してボタンを戻す
  useEffect(() => {
    if (cooldownUntil === null) return
    const timer = setTimeout(() => setCooldownUntil(null), Math.max(0, cooldownUntil - Date.now()))
    return () => clearTimeout(timer)
  }, [cooldownUntil])

  function reload() {
    if (cooldownUntil !== null) return
    setCooldownUntil(Date.now() + RELOAD_COOLDOWN_MS)
    // 失敗しても前回の一覧を保ち、次の再読み込みに任せる(useSuspenseQuery の再取得は throw しない)
    void queryClient.refetchQueries({ queryKey })
  }

  return { reload, isDisabled: isFetching || cooldownUntil !== null, isFetching }
}
