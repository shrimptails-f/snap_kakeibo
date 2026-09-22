import type { AnalysisRequestItem } from '../types/analysis-request.types'

// design_guidelines.md §5 の色の役割
export type StatusTone = 'neutral' | 'info' | 'primary' | 'warning' | 'danger'

export type AnalysisRequestStatusView = {
  label: string
  tone: StatusTone
  isStalled?: boolean
}

export const ANALYSIS_STALLED_AFTER_MS = 30 * 60 * 1000

// 解析依頼の表示名と色の役割(docs/screens/analysis-requests)。UPLOADING は画面側で期限切れを判定する
export function analysisRequestStatus(item: AnalysisRequestItem, now: number = Date.now()): AnalysisRequestStatusView {
  if (item.status === 'UPLOADING') {
    return now > new Date(item.upload_expires_at).getTime()
      ? { label: '期限切れ', tone: 'warning' }
      : { label: 'アップロード待ち', tone: 'neutral' }
  }
  if (item.status === 'ANALYZING' && now - new Date(item.updated_at).getTime() > ANALYSIS_STALLED_AFTER_MS) {
    return { label: '停滞', tone: 'warning', isStalled: true }
  }
  return SETTLED_STATUS_VIEWS[item.status]
}

const FAILURE_MESSAGES: Record<string, string> = {
  ANALYSIS_FAILED: '画像を解析できませんでした。',
  INVALID_DATE: '購入日を正しく読み取れませんでした。',
  INVALID_AMOUNT: '金額を正しく読み取れませんでした。',
  NO_TOTAL_AMOUNT: '合計金額を読み取れませんでした。',
  NO_DATE: '購入日を読み取れませんでした。',
  TOO_MANY_DETAILS: '商品明細が50件を超えています。',
  INTERNAL: 'システムエラーが発生しました。',
}

export function analysisRequestMessage(item: AnalysisRequestItem, now: number = Date.now()): string | null {
  const view = analysisRequestStatus(item, now)
  if (view.label === '期限切れ') return '送信期限を過ぎました。新しい取り込みとして画像を選び直してください。'
  if (view.isStalled) return '解析に時間がかかっています。'
  if (item.status === 'UPLOADING') return '送信済み・解析開始待ちです。'
  if (item.status === 'ANALYZING') return '結果は再読み込みで確認できます。'
  if (item.status === 'NO_DATA') return '登録できる明細が見つかりませんでした。家計簿には反映されていません。'
  if (item.status === 'FAILED') return FAILURE_MESSAGES[item.error_code ?? ''] ?? '画像を解析できませんでした。'
  return null
}

export function canRetryAnalysis(item: AnalysisRequestItem, now: number = Date.now()): boolean {
  if (item.error_code === 'TOO_MANY_DETAILS') return false
  return item.status === 'FAILED' || item.status === 'NO_DATA' || analysisRequestStatus(item, now).isStalled === true
}

const SETTLED_STATUS_VIEWS: Record<Exclude<AnalysisRequestItem['status'], 'UPLOADING'>, AnalysisRequestStatusView> = {
  ANALYZING: { label: '解析中', tone: 'info' },
  SUCCEEDED: { label: '登録完了', tone: 'primary' },
  NO_DATA: { label: '登録対象なし', tone: 'neutral' },
  FAILED: { label: '解析失敗', tone: 'danger' },
}
