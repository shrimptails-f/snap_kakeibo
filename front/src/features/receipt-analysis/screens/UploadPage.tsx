import { useEffect, useMemo, useRef, useState } from 'react'
import type { ChangeEvent } from 'react'
import { Link, useBeforeUnload, useBlocker, useSearchParams } from 'react-router'
import { toFriendlyMessage } from '@/shared/api/errors'
import { Button } from '@/shared/ui/Button'
import { ReceiptPreviewDialog } from '../components/ReceiptPreviewDialog'
import { RetryAnalysisDialog } from '../components/RetryAnalysisDialog'
import { useAnalysisRequests } from '../hooks/useAnalysisRequests'
import { useReceiptUploadBatch } from '../hooks/useReceiptUploadBatch'
import type { ReceiptUpload, SelectedReceipt } from '../hooks/useReceiptUploadBatch'
import { useReloadAnalysisRequests } from '../hooks/useReloadAnalysisRequests'
import { useRetryAnalysis } from '../hooks/useRetryAnalysis'
import { analysisRequestMessage, analysisRequestStatus, canRetryAnalysis } from '../lib/analysisRequestStatus'
import type { AnalysisRequestItem } from '../types/analysis-request.types'
import styles from './UploadPage.module.css'

function currentMonth(): string {
  return new Date().toISOString().slice(0, 7)
}

type Preview = { fileName: string; previewUrl: string }

function ReceiptThumbnail({ item, onPreview }: { item: SelectedReceipt; onPreview: (preview: Preview) => void }) {
  return (
    <button className={styles.thumbnailButton} type="button" onClick={() => onPreview({ fileName: item.file.name, previewUrl: item.previewUrl })}>
      <img src={item.previewUrl} alt="" />
      <span className={styles.srOnly}>{item.file.name} を拡大</span>
    </button>
  )
}

function serverStatus(upload: ReceiptUpload, request?: AnalysisRequestItem) {
  if (upload.phase === 'creating' || upload.phase === 'uploading') return { label: 'アップロード中', tone: 'info' as const }
  if (upload.phase === 'failed' && (!request || request.status === 'UPLOADING')) return { label: '送信失敗', tone: 'danger' as const }
  if (request) return analysisRequestStatus(request)
  if (upload.phase === 'sent') return { label: '送信済み', tone: 'neutral' as const }
  return { label: '送信失敗', tone: 'danger' as const }
}

export function UploadPage() {
  const month = currentMonth()
  const [searchParams] = useSearchParams()
  const { data: requests, dataUpdatedAt } = useAnalysisRequests(month)
  const reload = useReloadAnalysisRequests(month)
  const retry = useRetryAnalysis(month)
  const batch = useReceiptUploadBatch(month)
  const [preview, setPreview] = useState<Preview | null>(null)
  const [retryTarget, setRetryTarget] = useState<AnalysisRequestItem | null>(null)
  const [retryError, setRetryError] = useState<string | null>(null)
  const stayButtonRef = useRef<HTMLButtonElement>(null)
  const moveButtonRef = useRef<HTMLButtonElement>(null)
  const hasUnfinishedTransfer = batch.selected.length > 0 || batch.isUploading
  const navigationBlocker = useBlocker(({ currentLocation, nextLocation }) =>
    hasUnfinishedTransfer && currentLocation.pathname !== nextLocation.pathname,
  )
  useBeforeUnload((event) => {
    if (!hasUnfinishedTransfer) return
    event.preventDefault()
    event.returnValue = ''
  })
  useEffect(() => {
    if (navigationBlocker.state !== 'blocked') return
    stayButtonRef.current?.focus()
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') navigationBlocker.reset?.()
      if (event.key === 'Tab') {
        event.preventDefault()
        const next = event.shiftKey || document.activeElement === moveButtonRef.current ? stayButtonRef.current : moveButtonRef.current
        next?.focus()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [navigationBlocker])
  const requestById = useMemo(
    () => new Map(requests.items.map((item) => [item.analysis_request_id, item])),
    [requests.items],
  )
  const returnedRequest = requestById.get(searchParams.get('request') ?? '')

  function handleFiles(event: ChangeEvent<HTMLInputElement>) {
    if (event.target.files?.length) batch.addFiles(event.target.files)
    event.target.value = ''
  }

  async function handleRetry() {
    if (!retryTarget) return
    setRetryError(null)
    try {
      await retry.mutateAsync(retryTarget.analysis_request_id)
      setRetryTarget(null)
    } catch (error: unknown) {
      setRetryError(toFriendlyMessage(error))
      setRetryTarget(null)
    }
  }

  const counts = batch.uploads.reduce(
    (result, upload) => {
      const request = upload.analysisRequestId ? requestById.get(upload.analysisRequestId) : undefined
      const status = serverStatus(upload, request).label
      if (status === '登録完了') result.completed += 1
      else if (status === '登録対象なし') result.noData += 1
      else if (['送信失敗', '解析失敗', '期限切れ', '停滞'].includes(status)) result.needsAction += 1
      else result.inProgress += 1
      return result
    },
    { completed: 0, inProgress: 0, noData: 0, needsAction: 0 },
  )
  const summary = [
    counts.completed > 0 ? `登録完了${counts.completed}` : null,
    counts.inProgress > 0 ? `進行中${counts.inProgress}` : null,
    counts.noData > 0 ? `登録対象なし${counts.noData}` : null,
    counts.needsAction > 0 ? `要対応${counts.needsAction}` : null,
  ].filter(Boolean).join(' / ')

  return (
    <div className={styles.page}>
      <div className={styles.pageHeader}>
        <div>
          <h1>レシートを取り込む</h1>
          <p className={styles.lead}>撮影するか、画像を選んでください。</p>
        </div>
        <Link className={styles.historyLink} to={`/analysis-requests?month=${month}`}>解析履歴を見る <span aria-hidden="true">›</span></Link>
      </div>

      {searchParams.get('file') && (
        <p className={styles.reuploadNotice} role="status">
          <strong>{searchParams.get('file')}</strong> の画像をもう一度選ぶか、撮り直してください。新しい取り込みとして登録します。
        </p>
      )}

      {returnedRequest && (
        <section className={styles.returnedRequest} aria-labelledby="returned-request-heading">
          <div>
            <h2 id="returned-request-heading">詳細から戻りました</h2>
            <strong>{returnedRequest.file_name}</strong>
            <p>{analysisRequestStatus(returnedRequest).label} — {analysisRequestMessage(returnedRequest) ?? '支出として登録されています。'}</p>
          </div>
          <Button variant="secondary" onClick={reload.reload} disabled={reload.isDisabled}>
            {reload.isFetching ? '再読み込み中…' : '状態を再読み込み'}
          </Button>
        </section>
      )}

      <section className={styles.picker} aria-labelledby="pick-heading">
        <h2 id="pick-heading">画像を選ぶ</h2>
        <div className={styles.pickActions}>
          <label className={styles.primaryFileAction}>
            <input type="file" accept="image/jpeg,image/png" capture="environment" onChange={handleFiles} disabled={batch.isUploading} />
            {batch.selected.length > 0 ? '追加で撮影' : 'レシートを撮影'}
          </label>
          <label className={styles.secondaryFileAction}>
            <input type="file" accept="image/jpeg,image/png" multiple onChange={handleFiles} disabled={batch.isUploading} />
            {batch.selected.length > 0 ? '画像を追加' : '画像を選ぶ（複数可）'}
          </label>
        </div>
        <p className={styles.hint}>JPEG・PNGに対応。1枚の画像につき1件ずつ解析します。日付と合計まで写してください。</p>
      </section>

      {batch.selected.length > 0 && (
        <section className={styles.section} aria-labelledby="selected-heading">
          <div className={styles.sectionHeader}>
            <h2 id="selected-heading">選択した画像 <span>{batch.selected.length}枚</span></h2>
          </div>
          <ul className={styles.receiptList}>
            {batch.selected.map((item) => (
              <li className={styles.receiptRow} key={item.localId}>
                <ReceiptThumbnail item={item} onPreview={setPreview} />
                <div className={styles.fileInfo}>
                  <strong>{item.file.name}</strong>
                  <span className={item.validationError ? styles.statusDanger : styles.statusNeutral}>
                    {item.validationError ? '× 形式を確認' : '○ 選択済み'}
                  </span>
                  {item.validationError && <p className={styles.errorText}>{item.validationError}</p>}
                </div>
                <Button variant="secondary" onClick={() => batch.removeSelected(item.localId)} disabled={batch.isUploading}>
                  外す
                </Button>
              </li>
            ))}
          </ul>
          <div className={styles.uploadAction}>
            <Button variant="primary" onClick={batch.uploadSelected} disabled={batch.validCount === 0 || batch.isUploading}>
              {batch.isUploading ? 'アップロード中…' : `${batch.validCount}枚をアップロード`}
            </Button>
            {batch.validCount < batch.selected.length && <p>形式が合わない画像は送信しません。</p>}
          </div>
        </section>
      )}

      {batch.uploads.length > 0 && (
        <section className={styles.section} aria-labelledby="current-heading">
          <div className={styles.progressHeader}>
            <div>
              <h2 id="current-heading">今回の取り込み <span>{batch.uploads.length}枚</span></h2>
              <p className={styles.summary}>{summary}</p>
              <p className={styles.checkedAt}>状態確認 {new Date(dataUpdatedAt).toLocaleTimeString('ja-JP', { hour: '2-digit', minute: '2-digit' })}</p>
            </div>
            <Button variant="secondary" onClick={reload.reload} disabled={reload.isDisabled}>
              {reload.isFetching ? '再読み込み中…' : '再読み込み'}
            </Button>
          </div>
          {retryError && <p className={styles.errorNotice} role="alert">{retryError} 最新の状態を再読み込みしてから、もう一度お試しください。</p>}
          {reload.hasError && <p className={styles.errorNotice} role="alert">最新の状態を取得できませんでした。前回確認した状態を表示しています。</p>}
          {!batch.isUploading && batch.uploads.every((item) => item.phase === 'sent') && (
            <p className={styles.sentNotice} role="status">画像の送信が終わりました。画面を離れても解析は続きます。結果は解析履歴から確認できます。</p>
          )}
          <ul className={styles.receiptList}>
            {batch.uploads.map((upload) => {
              const request = upload.analysisRequestId ? requestById.get(upload.analysisRequestId) : undefined
              const status = serverStatus(upload, request)
              const isUnconfirmedUploadFailure = upload.phase === 'failed' && (!request || request.status === 'UPLOADING')
              const message = isUnconfirmedUploadFailure ? upload.errorMessage : request ? analysisRequestMessage(request) : upload.errorMessage
              return (
                <li className={styles.receiptRow} id={`upload-${upload.localId}`} key={upload.localId}>
                  <ReceiptThumbnail item={upload} onPreview={setPreview} />
                  <div className={styles.fileInfo}>
                    <strong>{upload.file.name}</strong>
                    <span className={styles[`status${status.tone[0].toUpperCase()}${status.tone.slice(1)}`]}>{status.label}</span>
                    {message && <p>{message}</p>}
                  </div>
                  <div className={styles.rowActions}>
                    {request?.status === 'SUCCEEDED' && request.expense_id && (
                      <Link to={`/expenses/${encodeURIComponent(request.expense_id)}?from=upload&request=${encodeURIComponent(request.analysis_request_id)}`}>
                        支出詳細を見る <span aria-hidden="true">›</span>
                      </Link>
                    )}
                    {request && canRetryAnalysis(request) && (
                      <Button variant="secondary" onClick={() => setRetryTarget(request)}>再解析する</Button>
                    )}
                    {request && (analysisRequestStatus(request).label === '期限切れ' || request.error_code === 'TOO_MANY_DETAILS') && (
                      <Link to={`/upload?file=${encodeURIComponent(request.file_name)}`}>撮り直してアップロード <span aria-hidden="true">›</span></Link>
                    )}
                    {upload.phase === 'failed' && !upload.analysisRequestId && (
                      <Button variant="secondary" onClick={() => batch.retryCreating(upload.localId)} disabled={batch.isUploading}>再試行</Button>
                    )}
                    {isUnconfirmedUploadFailure && upload.analysisRequestId && (
                      <Button variant="secondary" onClick={reload.reload} disabled={reload.isDisabled}>状態を確認</Button>
                    )}
                  </div>
                </li>
              )
            })}
          </ul>
        </section>
      )}

      {preview && <ReceiptPreviewDialog {...preview} onClose={() => setPreview(null)} />}
      {retryTarget && (
        <RetryAnalysisDialog
          fileName={retryTarget.file_name}
          isStalled={analysisRequestStatus(retryTarget).isStalled === true}
          isPending={retry.isPending}
          onCancel={() => setRetryTarget(null)}
          onConfirm={handleRetry}
        />
      )}
      {navigationBlocker.state === 'blocked' && (
        <div className={styles.dialogBackdrop} role="presentation">
          <div className={styles.leaveDialog} role="alertdialog" aria-modal="true" aria-labelledby="leave-title">
            <h2 id="leave-title">この画面を移動しますか？</h2>
            <p>未送信の画像があります。移動すると送信が中断されることがあります。</p>
            <div className={styles.leaveActions}>
              <Button ref={stayButtonRef} variant="secondary" onClick={() => navigationBlocker.reset?.()}>この画面に戻る</Button>
              <Button ref={moveButtonRef} variant="danger" onClick={() => navigationBlocker.proceed?.()}>移動する</Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
