import { startTransition, Suspense, useEffect, useRef, useState } from 'react'
import { useQueryClient, useQueryErrorResetBoundary } from '@tanstack/react-query'
import { Link, useLocation, useNavigate, useSearchParams } from 'react-router'
import { toFriendlyMessage } from '@/shared/api/errors'
import { formatYen } from '@/shared/lib/formatYen'
import { Button } from '@/shared/ui/Button'
import { ReloadButton } from '@/shared/ui/ReloadButton'
import { ErrorBoundary } from '@/shared/ui/ErrorBoundary'
import { SpinnerBlock } from '@/shared/ui/Spinner'
import { StatusBadge } from '@/shared/ui/StatusBadge'
import { InfoTooltip } from '@/shared/ui/InfoTooltip'
import { RetryAnalysisDialog } from '../components/RetryAnalysisDialog'
import { listAnalysisRequests } from '../api/analysis-requests.api'
import { analysisRequestsQueryKey, useAnalysisRequests } from '../hooks/useAnalysisRequests'
import { useReloadAnalysisRequests } from '../hooks/useReloadAnalysisRequests'
import { useRetryAnalysis } from '../hooks/useRetryAnalysis'
import { analysisRequestMessage, analysisRequestStatus, canRetryAnalysis } from '../lib/analysisRequestStatus'
import type { AnalysisRequestFilter, AnalysisRequestItem } from '../types/analysis-request.types'
import styles from './AnalysisRequestsPage.module.css'

const FILTER_LABELS: Record<AnalysisRequestFilter, string> = {
  all: 'すべて',
  attention: '要対応',
  in_progress: '進行中',
  succeeded: '登録完了',
  no_data: '登録対象なし',
}

type RestoreState = {
  restoreRequestId?: string
  scrollY?: number
  analysisCursorHistory?: string[]
}

type Notice = { message: string; showProgressLink?: boolean }

function currentMonth(): string {
  const now = new Date()
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
}

function requestedMonth(value: string | null): string {
  return value && /^\d{4}-(0[1-9]|1[0-2])$/.test(value) ? value : currentMonth()
}

function requestedFilter(value: string | null): AnalysisRequestFilter {
  return value && value in FILTER_LABELS ? value as AnalysisRequestFilter : 'all'
}

function requestedPage(value: string | null, hasCursor: boolean): number {
  const page = Number(value)
  return hasCursor && Number.isInteger(page) && page > 1 ? page : 1
}

function historySearch(month: string, filter: AnalysisRequestFilter, cursor = '', page = 1): string {
  const search = new URLSearchParams({ month, filter })
  if (cursor) search.set('cursor', cursor)
  if (page > 1) search.set('page', String(page))
  return `?${search}`
}

function AnalysisRequestsContent() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const location = useLocation()
  const queryClient = useQueryClient()
  const month = requestedMonth(searchParams.get('month'))
  const filter = requestedFilter(searchParams.get('filter'))
  const cursor = searchParams.get('cursor') ?? ''
  const page = requestedPage(searchParams.get('page'), cursor !== '')
  const restoreState = location.state as RestoreState | null
  const cursorHistory = restoreState?.analysisCursorHistory ?? []
  const { data: requests, dataUpdatedAt, isFetching } = useAnalysisRequests(month, filter, cursor)
  const reload = useReloadAnalysisRequests(month, filter, cursor)
  const retry = useRetryAnalysis(month)
  const [retryTarget, setRetryTarget] = useState<AnalysisRequestItem | null>(null)
  const [notice, setNotice] = useState<Notice | null>(null)
  const [pageError, setPageError] = useState<string | null>(null)
  const [pendingSearch, setPendingSearch] = useState<string | null>(null)
  const didRestorePosition = useRef(false)
  const isPageTransitioning = pendingSearch !== null && pendingSearch !== location.search
  const isBusy = isFetching || isPageTransitioning

  useEffect(() => {
    const requestId = restoreState?.restoreRequestId
    if (!requestId || didRestorePosition.current) return
    didRestorePosition.current = true
    requestAnimationFrame(() => {
      const row = document.getElementById(`analysis-request-${requestId}`)
      row?.focus()
      if (!row && Number.isFinite(restoreState.scrollY)) window.scrollTo({ top: restoreState.scrollY })
    })
  }, [restoreState, requests.items])

  async function loadAndMove(nextMonth: string, nextFilter: AnalysisRequestFilter, nextCursor: string, nextPageNumber: number, nextCursorHistory: string[]) {
    const search = historySearch(nextMonth, nextFilter, nextCursor, nextPageNumber)
    setPendingSearch(search)
    setPageError(null)
    try {
      await queryClient.fetchQuery({
        queryKey: analysisRequestsQueryKey(nextMonth, nextFilter, nextCursor),
        queryFn: ({ signal }) => listAnalysisRequests(nextMonth, nextFilter, nextCursor, signal),
        staleTime: 0,
      })
    } catch {
      setPendingSearch(location.search)
      setPageError('解析履歴を読み込めませんでした。元のページを表示しています。もう一度操作してください。')
      return
    }
    startTransition(() => navigate({ pathname: '/analysis-requests', search }, { state: { analysisCursorHistory: nextCursorHistory } }))
  }

  function moveTo(nextMonth: string, nextFilter: AnalysisRequestFilter) {
    void loadAndMove(nextMonth, nextFilter, '', 1, [])
  }

  function nextPage() {
    if (!requests.next_cursor || isBusy) return
    void loadAndMove(month, filter, requests.next_cursor, page + 1, [...cursorHistory, cursor])
  }

  function previousPage() {
    if (isBusy || page === 1) return
    const previousCursor = cursorHistory.at(-1)
    const previousPageNumber = page - 1
    void loadAndMove(month, filter, previousCursor ?? '', previousCursor === undefined ? 1 : previousPageNumber, previousCursor === undefined ? [] : cursorHistory.slice(0, -1))
  }

  async function handleRetry() {
    if (!retryTarget) return
    try {
      await retry.mutateAsync(retryTarget.analysis_request_id)
      setNotice({ message: '再解析を開始しました。結果は再読み込みで確認できます。', showProgressLink: filter === 'attention' })
    } catch (error: unknown) {
      setNotice({ message: `${toFriendlyMessage(error)} 最新の状態を再読み込みしてください。` })
    } finally {
      setRetryTarget(null)
    }
  }

  const [year, monthNumber] = month.split('-')
  const monthLabel = `${year}年${Number(monthNumber)}月`
  const updatedAt = dataUpdatedAt ? new Date(dataUpdatedAt).toLocaleTimeString('ja-JP', { hour: '2-digit', minute: '2-digit' }) : '確認中'

  return (
    <div className={styles.page}>
      <div className={styles.pageHeader}>
        <div><h1>解析履歴</h1><p>取り込んだ画像の状態を受付月から確認できます。</p></div>
        <Link className={styles.uploadLink} to="/upload">レシートを取り込む</Link>
      </div>

      <section className={styles.search} aria-labelledby="analysis-search-heading">
        <h2 id="analysis-search-heading">検索条件</h2>
        <div className={styles.searchFields}>
          <label>受付月<input className="field-control" type="month" value={month} onChange={(event) => { if (event.target.value) moveTo(requestedMonth(event.target.value), filter) }} /></label>
          <label>表示<select className="field-control" value={filter} onChange={(event) => moveTo(month, event.target.value as AnalysisRequestFilter)}>{Object.entries(FILTER_LABELS).map(([value, label]) => <option value={value} key={value}>{label}</option>)}</select></label>
          <Link className={styles.attentionLink} to={`/analysis-requests${historySearch(month, 'attention')}`} onClick={(event) => { event.preventDefault(); moveTo(month, 'attention') }}>要対応を見る <span aria-hidden="true">›</span></Link>
        </div>
        <p>受付月はレシートの購入月とは異なります。</p>
      </section>

      <section className={styles.history} aria-labelledby="history-heading" aria-busy={isBusy}>
        <div className={styles.historyHeader}>
          <div>
            <h2 id="history-heading">履歴</h2>
            <p>{monthLabel} / {FILTER_LABELS[filter]}</p>
            <p>状態確認 {updatedAt}</p>
          </div>
          <ReloadButton reload={reload} />
        </div>
        {isBusy && <p className={styles.loadingNotice} role="status">{page > 1 ? `${page}ページ目を読み込み中…` : `${monthLabel}の履歴を読み込み中…`}</p>}
        {notice && <p className={styles.notice} role="status">{notice.message} {notice.showProgressLink && <Link to={`/analysis-requests${historySearch(month, 'in_progress')}`} onClick={(event) => { event.preventDefault(); moveTo(month, 'in_progress') }}>進行中で確認する</Link>}</p>}
        {pageError && <p className={styles.errorNotice} role="alert">{pageError}</p>}
        {reload.hasError && <p className={styles.errorNotice} role="alert">最新の状態を取得できませんでした。前回確認した一覧を表示しています。</p>}
        {requests.items.length === 0 ? (
          <div className={styles.empty}>
            <p>{filter === 'all' ? 'この受付月の取り込みはありません。' : `${FILTER_LABELS[filter]}の画像はありません。`}</p>
            {filter === 'all' ? <Link to="/upload">レシートを取り込む</Link> : <Link to={`/analysis-requests${historySearch(month, 'all')}`} onClick={(event) => { event.preventDefault(); moveTo(month, 'all') }}>すべてを見る</Link>}
          </div>
        ) : (
          <div className={styles.tableScroll}>
            <table className={styles.table}>
              <thead><tr><th scope="col">受付日時</th><th scope="col">ファイル名</th><th scope="col">状態・理由</th><th scope="col"><InfoTooltip label="店舗・計上額" description="計上額は最終支払合計に利用者調整額を反映した金額です。明細合計とは一致しない場合があります。" /></th><th scope="col">操作</th></tr></thead>
              <tbody>{requests.items.map((item) => {
                const status = analysisRequestStatus(item)
                const message = analysisRequestMessage(item)
                const detailPath = `/expenses/${encodeURIComponent(item.expense_id ?? '')}`
                return (
                  <tr id={`analysis-request-${item.analysis_request_id}`} tabIndex={-1} key={item.analysis_request_id}>
                    <td data-label="受付日時"><time dateTime={item.created_at}>{new Date(item.created_at).toLocaleString('ja-JP')}</time></td>
                    <td data-label="ファイル名"><strong>{item.file_name}</strong></td>
                    <td data-label="状態・理由"><StatusBadge tone={status.tone}>{status.label}</StatusBadge>{message && <p>{message}</p>}</td>
                    <td className={styles.recordedCell}><span className={styles.mobileRecordedLabel}><InfoTooltip label="店舗・計上額" description="計上額は最終支払合計に利用者調整額を反映した金額です。明細合計とは一致しない場合があります。" /></span>{item.store_name !== undefined || item.recorded_amount !== undefined ? <><span>{item.store_name ?? '店舗名未取得'}</span>{item.recorded_amount !== undefined && <strong className="amount">{formatYen(item.recorded_amount)}</strong>}</> : <span className={styles.muted}>—</span>}</td>
                    <td data-label="操作" className={styles.actions}>
                      {!isBusy && item.status === 'SUCCEEDED' && item.expense_id && <Link to={detailPath} state={{ from: `${location.pathname}${location.search}`, backLabel: '解析履歴へ', requestId: item.analysis_request_id, scrollY: window.scrollY, analysisCursorHistory: cursorHistory }}>支出詳細を見る <span aria-hidden="true">›</span></Link>}
                      {!isBusy && canRetryAnalysis(item) && <Button variant="secondary" aria-label={`${item.file_name}を再解析する`} onClick={() => setRetryTarget(item)}>再解析する</Button>}
                      {!isBusy && (status.label === '期限切れ' || item.error_code === 'TOO_MANY_DETAILS') && <Link to={`/upload?file=${encodeURIComponent(item.file_name)}`}>撮り直す <span aria-hidden="true">›</span></Link>}
                      {isBusy && <span className={styles.muted}>読み込み中</span>}
                    </td>
                  </tr>
                )
              })}</tbody>
            </table>
          </div>
        )}

        {(page > 1 || requests.next_cursor) && <nav className={styles.pagination} aria-label="解析履歴のページ切り替え">
          <Button variant="secondary" disabled={page === 1 || isBusy} onClick={previousPage}>{cursorHistory.length === 0 && page > 1 ? '先頭へ戻る' : '前へ'}</Button>
          <span aria-current="page">{page}ページ目</span>
          <Button variant="secondary" disabled={!requests.next_cursor || isBusy} onClick={nextPage}>次へ</Button>
        </nav>}
      </section>
      {retryTarget && <RetryAnalysisDialog fileName={retryTarget.file_name} isStalled={analysisRequestStatus(retryTarget).isStalled === true} isPending={retry.isPending} onCancel={() => setRetryTarget(null)} onConfirm={handleRetry} />}
    </div>
  )
}

function InitialState({ error, onRetry }: { error?: boolean; onRetry?: () => void }) {
  return <div className={styles.page}>
    <div className={styles.pageHeader}>
      <div><h1>解析履歴</h1><p>取り込んだ画像の状態を受付月から確認できます。</p></div>
      <Link className={styles.uploadLink} to="/upload">レシートを取り込む</Link>
    </div>
    {error ? <section className={styles.initialState} role="alert"><h2>解析履歴を読み込めませんでした</h2><p>時間をおいて、もう一度お試しください。</p><Button variant="secondary" onClick={onRetry}>再読み込み</Button></section> : <SpinnerBlock label="解析履歴を読み込み中" />}
  </div>
}

export function AnalysisRequestsPage() {
  const resetBoundary = useQueryErrorResetBoundary()
  return <ErrorBoundary onReset={resetBoundary.reset} fallback={(_error, reset) => <InitialState error onRetry={reset} />}>
    <Suspense fallback={<InitialState />}><AnalysisRequestsContent /></Suspense>
  </ErrorBoundary>
}
