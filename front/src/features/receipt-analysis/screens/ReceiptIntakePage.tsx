import { Suspense, useState } from 'react'
import type { ChangeEvent } from 'react'
import { useQueryErrorResetBoundary } from '@tanstack/react-query'
import { ExpenseDetailPanel } from '@/features/expenses'
import { toFriendlyMessage } from '@/shared/api/errors'
import { Button } from '@/shared/ui/Button'
import { ErrorBoundary } from '@/shared/ui/ErrorBoundary'
import { SpinnerBlock } from '@/shared/ui/Spinner'
import { useAnalysisRequests } from '../hooks/useAnalysisRequests'
import { useReloadAnalysisRequests } from '../hooks/useReloadAnalysisRequests'
import { useUploadReceipt } from '../hooks/useUploadReceipt'
import { analysisRequestStatus } from '../lib/analysisRequestStatus'
import type { StatusTone } from '../lib/analysisRequestStatus'
import styles from './ReceiptIntakePage.module.css'

// 移行途中の検証画面。アップロード、解析依頼一覧、支出の表示を一つに持つ。
// 月の選択(URL 化)と支出詳細の別画面化は次の段階で行う

// 状態バッジの色。neutral は基本の .status だけ
const STATUS_TONE_CLASS: Record<StatusTone, string> = {
  neutral: styles.status,
  info: `${styles.status} ${styles.statusInfo}`,
  primary: `${styles.status} ${styles.statusPrimary}`,
  warning: `${styles.status} ${styles.statusWarning}`,
  danger: `${styles.status} ${styles.statusDanger}`,
}

function currentMonth(): string {
  return new Date().toISOString().slice(0, 7)
}

export function ReceiptIntakePage() {
  const [month] = useState(currentMonth)
  const [selectedExpenseId, setSelectedExpenseId] = useState<string | null>(null)
  // 初回は AppLayout の Suspense が待つ。再読み込み中は前回の一覧を保ち、失敗しても次回に任せる
  const { data: requests } = useAnalysisRequests(month)
  const reloadRequests = useReloadAnalysisRequests(month)
  const upload = useUploadReceipt()
  const queryErrorReset = useQueryErrorResetBoundary()

  function handleFileChange(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (file) upload.mutate({ file, yearMonth: month })
    e.target.value = ''
  }

  return (
    <>
      <div className={styles.pageHeader}>
        <h1 className={styles.title}>{month} のレシート取り込み</h1>
        <div className={styles.actions}>
          {/* 解析は非同期に進むので、利用者が再読み込みで確かめる(定期取得はしない) */}
          <Button variant="secondary" onClick={reloadRequests.reload} disabled={reloadRequests.isDisabled}>
            {reloadRequests.isFetching ? '再読み込み中...' : '再読み込み'}
          </Button>
          <label className={styles.uploadButton}>
            <input type="file" accept="image/*,.pdf" onChange={handleFileChange} disabled={upload.isPending} />
            {upload.isPending ? 'アップロード中...' : 'ファイルを選択'}
          </label>
        </div>
      </div>

      {upload.isSuccess && (
        <p className={styles.notice} role="status">
          アップロードしました。解析が完了すると解析依頼の一覧に反映されます。
        </p>
      )}
      {upload.isError && (
        <p className={styles.error} role="alert">
          {toFriendlyMessage(upload.error)}
        </p>
      )}

      <section className={styles.layout}>
        <div className={styles.panel}>
          <h2>解析依頼</h2>
          <div className={styles.list}>
            {requests.items.length === 0 && <p className="muted">まだ解析依頼がありません。</p>}
            {requests.items.map((item) => {
              const status = analysisRequestStatus(item)
              return (
                <button
                  className={styles.row}
                  key={item.analysis_request_id}
                  type="button"
                  disabled={!item.expense_id}
                  onClick={() => item.expense_id && setSelectedExpenseId(item.expense_id)}
                >
                  <span className={styles.rowBody}>
                    <strong>{item.file_name || item.analysis_request_id}</strong>
                    <small>{new Date(item.created_at).toLocaleString('ja-JP')}</small>
                    <small>試行 {item.attempt} 回目</small>
                    {item.error_message && <small>{item.error_message}</small>}
                    {item.failed_at && <small>失敗日時: {new Date(item.failed_at).toLocaleString('ja-JP')}</small>}
                  </span>
                  <span className={STATUS_TONE_CLASS[status.tone]}>{status.label}</span>
                </button>
              )
            })}
          </div>
        </div>

        <div className={styles.panel}>
          <h2>支出</h2>
          {!selectedExpenseId && <p className="muted">登録完了した解析依頼を選択してください。</p>}
          {selectedExpenseId && (
            // パネル単位で待つ・回復する。一覧は表示したまま、支出の取得中だけここにスピナーを出し、
            // 取得に失敗してもこのパネルの中で再試行できる
            <ErrorBoundary
              key={selectedExpenseId}
              onReset={queryErrorReset.reset}
              fallback={(error, reset) => (
                <div className={styles.panelError}>
                  <p role="alert">{toFriendlyMessage(error)}</p>
                  <Button variant="secondary" onClick={reset}>
                    再試行
                  </Button>
                </div>
              )}
            >
              <Suspense fallback={<SpinnerBlock label="支出を読み込んでいます" />}>
                <ExpenseDetailPanel expenseId={selectedExpenseId} />
                <a className={styles.detailLink} href={`/expenses/${encodeURIComponent(selectedExpenseId)}?from=upload`}>
                  支出詳細を開く
                </a>
              </Suspense>
            </ErrorBoundary>
          )}
        </div>
      </section>
    </>
  )
}
