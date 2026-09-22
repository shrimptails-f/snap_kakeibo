import { useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { toFriendlyMessage } from '@/shared/api/errors'
import { Button } from '@/shared/ui/Button'
import { RetryAnalysisDialog } from '../components/RetryAnalysisDialog'
import { useAnalysisRequests } from '../hooks/useAnalysisRequests'
import { useReloadAnalysisRequests } from '../hooks/useReloadAnalysisRequests'
import { useRetryAnalysis } from '../hooks/useRetryAnalysis'
import { analysisRequestMessage, analysisRequestStatus, canRetryAnalysis } from '../lib/analysisRequestStatus'
import type { AnalysisRequestItem } from '../types/analysis-request.types'
import styles from './AnalysisRequestsPage.module.css'

function requestedMonth(value: string | null): string {
  return value && /^\d{4}-(0[1-9]|1[0-2])$/.test(value) ? value : new Date().toISOString().slice(0, 7)
}

export function AnalysisRequestsPage() {
  const [searchParams] = useSearchParams()
  const month = requestedMonth(searchParams.get('month'))
  const { data: requests, dataUpdatedAt } = useAnalysisRequests(month)
  const reload = useReloadAnalysisRequests(month)
  const retry = useRetryAnalysis(month)
  const [retryTarget, setRetryTarget] = useState<AnalysisRequestItem | null>(null)
  const [notice, setNotice] = useState<string | null>(null)

  async function handleRetry() {
    if (!retryTarget) return
    try {
      await retry.mutateAsync(retryTarget.analysis_request_id)
      setNotice('再解析を開始しました。結果は再読み込みで確認できます。')
    } catch (error: unknown) {
      setNotice(`${toFriendlyMessage(error)} 最新の状態を再読み込みしてください。`)
    } finally {
      setRetryTarget(null)
    }
  }

  return (
    <div className={styles.page}>
      <div className={styles.pageHeader}>
        <div><h1>解析履歴</h1><p>{month.replace('-', '年')}月に受け付けた画像</p></div>
        <Link className={styles.uploadLink} to="/upload">レシートを取り込む</Link>
      </div>
      <section className={styles.history} aria-labelledby="history-heading">
        <div className={styles.historyHeader}>
          <div>
            <h2 id="history-heading">履歴</h2>
            <p>状態確認 {new Date(dataUpdatedAt).toLocaleTimeString('ja-JP', { hour: '2-digit', minute: '2-digit' })}</p>
          </div>
          <Button variant="secondary" onClick={reload.reload} disabled={reload.isDisabled}>
            {reload.isFetching ? '再読み込み中…' : '再読み込み'}
          </Button>
        </div>
        {notice && <p className={styles.notice} role="status">{notice}</p>}
        {reload.hasError && <p className={styles.errorNotice} role="alert">最新の状態を取得できませんでした。前回確認した一覧を表示しています。</p>}
        {requests.items.length === 0 ? (
          <div className={styles.empty}><p>この受付月の取り込みはありません。</p><Link to="/upload">レシートを取り込む</Link></div>
        ) : (
          <ul className={styles.list}>
            {requests.items.map((item) => {
              const status = analysisRequestStatus(item)
              const message = analysisRequestMessage(item)
              return (
                <li className={styles.row} key={item.analysis_request_id}>
                  <div className={styles.identity}>
                    <time dateTime={item.created_at}>{new Date(item.created_at).toLocaleString('ja-JP')}</time>
                    <strong>{item.file_name}</strong>
                  </div>
                  <div className={styles.state}>
                    <span className={styles[`status${status.tone[0].toUpperCase()}${status.tone.slice(1)}`]}>{status.label}</span>
                    {message && <p>{message}</p>}
                  </div>
                  <div className={styles.actions}>
                    {item.status === 'SUCCEEDED' && item.expense_id && <Link to={`/expenses/${encodeURIComponent(item.expense_id)}?from=analysis-requests`}>支出詳細を見る <span aria-hidden="true">›</span></Link>}
                    {canRetryAnalysis(item) && <Button variant="secondary" onClick={() => setRetryTarget(item)}>再解析する</Button>}
                    {(status.label === '期限切れ' || item.error_code === 'TOO_MANY_DETAILS') && <Link to={`/upload?file=${encodeURIComponent(item.file_name)}`}>撮り直す <span aria-hidden="true">›</span></Link>}
                  </div>
                </li>
              )
            })}
          </ul>
        )}
      </section>
      {retryTarget && <RetryAnalysisDialog fileName={retryTarget.file_name} isStalled={analysisRequestStatus(retryTarget).isStalled === true} isPending={retry.isPending} onCancel={() => setRetryTarget(null)} onConfirm={handleRetry} />}
    </div>
  )
}
