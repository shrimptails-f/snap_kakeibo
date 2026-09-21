import type { AnalysisRequestItem } from '../types/analysis-request.types'

// design_guidelines.md §5 の色の役割
export type StatusTone = 'neutral' | 'info' | 'primary' | 'warning' | 'danger'

export type AnalysisRequestStatusView = {
  label: string
  tone: StatusTone
}

// 解析依頼の表示名と色の役割(docs/screens/analysis-requests)。UPLOADING は画面側で期限切れを判定する
export function analysisRequestStatus(item: AnalysisRequestItem, now: number = Date.now()): AnalysisRequestStatusView {
  if (item.status === 'UPLOADING') {
    return now > new Date(item.upload_expires_at).getTime()
      ? { label: '期限切れ', tone: 'warning' }
      : { label: 'アップロード待ち', tone: 'neutral' }
  }
  return SETTLED_STATUS_VIEWS[item.status]
}

const SETTLED_STATUS_VIEWS: Record<Exclude<AnalysisRequestItem['status'], 'UPLOADING'>, AnalysisRequestStatusView> = {
  ANALYZING: { label: '解析中', tone: 'info' },
  SUCCEEDED: { label: '登録完了', tone: 'primary' },
  NO_DATA: { label: '登録対象なし', tone: 'neutral' },
  FAILED: { label: '解析失敗', tone: 'danger' },
}
