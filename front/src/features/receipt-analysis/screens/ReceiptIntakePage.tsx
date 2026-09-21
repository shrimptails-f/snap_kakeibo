import { useEffect, useState } from 'react'
import type { ChangeEvent } from 'react'
import { categoryLabel, getExpense, sourceLabel } from '@/features/expenses'
import type { Expense, ExpenseDetail } from '@/features/expenses'
import { toFriendlyMessage } from '@/shared/api/errors'
import { formatYen } from '@/shared/lib/formatYen'
import { listAnalysisRequests } from '../api/analysis-requests.api'
import { createUpload, uploadToPresignedUrl } from '../api/uploads.api'
import { analysisRequestStatus } from '../lib/analysisRequestStatus'
import type { AnalysisRequestItem } from '../types/analysis-request.types'

// 移行途中の検証画面。アップロード、解析依頼一覧、支出の表示を一つに持つ。
// 月の選択(URL 化)、polling のサーバー状態管理層への移行、支出詳細の別画面化は次の段階で行う

const REFRESH_INTERVAL_MS = 5000

function currentMonth(): string {
  return new Date().toISOString().slice(0, 7)
}

export function ReceiptIntakePage() {
  const [month] = useState(currentMonth)
  const [requests, setRequests] = useState<AnalysisRequestItem[]>([])
  const [selectedExpenseId, setSelectedExpenseId] = useState<string | null>(null)
  const [expense, setExpense] = useState<Expense | null>(null)
  const [details, setDetails] = useState<ExpenseDetail[]>([])
  const [isUploading, setIsUploading] = useState(false)
  const [message, setMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  async function refreshRequests() {
    const data = await listAnalysisRequests(month)
    setRequests(data.items)
  }

  useEffect(() => {
    let isActive = true
    async function load(shouldReportError: boolean) {
      try {
        const data = await listAnalysisRequests(month)
        if (isActive) setRequests(data.items)
      } catch (e: unknown) {
        // 定期更新の失敗は次回に任せ、初回や操作時のエラー表示を上書きしない
        if (isActive && shouldReportError) setError(toFriendlyMessage(e))
      }
    }
    void load(true)
    const timer = window.setInterval(() => void load(false), REFRESH_INTERVAL_MS)
    return () => {
      isActive = false
      window.clearInterval(timer)
    }
  }, [month])

  useEffect(() => {
    if (!selectedExpenseId) return
    const controller = new AbortController()
    getExpense(selectedExpenseId, controller.signal)
      .then((data) => {
        setExpense(data.expense)
        setDetails(data.details)
      })
      .catch((e: unknown) => {
        if (controller.signal.aborted) return
        setError(toFriendlyMessage(e))
      })
    return () => controller.abort()
  }, [selectedExpenseId])

  async function upload(file: File) {
    setIsUploading(true)
    setError(null)
    setMessage(null)
    try {
      const contentType = file.type || 'image/jpeg'
      const created = await createUpload({ file_name: file.name, content_type: contentType })
      await uploadToPresignedUrl(created.put_url, file, contentType)
      setMessage('アップロードしました。解析が完了すると解析依頼の一覧に反映されます。')
      await refreshRequests()
    } catch (e: unknown) {
      setError(toFriendlyMessage(e))
    } finally {
      setIsUploading(false)
    }
  }

  function handleFileChange(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (file) void upload(file)
    e.target.value = ''
  }

  return (
    <>
      <div className="pageHeader">
        <p>{month} のレシート取り込み</p>
        <label className="uploadButton">
          <input type="file" accept="image/*,.pdf" onChange={handleFileChange} disabled={isUploading} />
          {isUploading ? 'アップロード中...' : 'ファイルを選択'}
        </label>
      </div>

      {message && (
        <p className="notice" role="status">
          {message}
        </p>
      )}
      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}

      <section className="layout">
        <div className="panel">
          <h2>解析依頼</h2>
          <div className="list">
            {requests.length === 0 && <p className="muted">まだ解析依頼がありません。</p>}
            {requests.map((item) => {
              const status = analysisRequestStatus(item)
              return (
                <button
                  className="row"
                  key={item.analysis_request_id}
                  type="button"
                  disabled={!item.expense_id}
                  onClick={() => item.expense_id && setSelectedExpenseId(item.expense_id)}
                >
                  <span>
                    <strong>{item.file_name || item.analysis_request_id}</strong>
                    <small>{new Date(item.created_at).toLocaleString('ja-JP')}</small>
                    <small>試行 {item.attempt} 回目</small>
                    {item.error_message && <small>{item.error_message}</small>}
                    {item.failed_at && <small>失敗日時: {new Date(item.failed_at).toLocaleString('ja-JP')}</small>}
                  </span>
                  <span className={`status status--${status.tone}`}>{status.label}</span>
                </button>
              )
            })}
          </div>
        </div>

        <div className="panel">
          <h2>支出</h2>
          {!expense && <p className="muted">登録完了した解析依頼を選択してください。</p>}
          {expense && (
            <>
              <div className="summary">
                <span>{expense.store_name}</span>
                <strong className="amount">{formatYen(expense.recorded_amount)}</strong>
                <small>{expense.purchase_date}</small>
                <small>
                  {sourceLabel(expense.source)}
                  {expense.is_edited && '・手動編集済み'} / 最終更新: {new Date(expense.updated_at).toLocaleString('ja-JP')}
                </small>
                {expense.adjustment_amount !== 0 && (
                  <small>
                    読取金額 {formatYen(expense.read_amount)} / 調整額 {formatYen(expense.adjustment_amount)}
                  </small>
                )}
              </div>
              <table>
                <thead>
                  <tr>
                    <th>品目</th>
                    <th>カテゴリ</th>
                    <th>金額</th>
                  </tr>
                </thead>
                <tbody>
                  {details.map((detail) => (
                    <tr key={detail.detail_id}>
                      <td>
                        {detail.name}
                        {detail.is_edited && <small>（手動編集済み）</small>}
                      </td>
                      <td>{categoryLabel(detail.category)}</td>
                      <td className="amount">{formatYen(detail.amount)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}
        </div>
      </section>
    </>
  )
}
