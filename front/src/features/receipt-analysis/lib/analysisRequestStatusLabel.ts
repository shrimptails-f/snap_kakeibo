import type { AnalysisRequestItem } from '../types/analysis-request.types'

// 解析依頼の表示名(docs/screens/analysis-requests)。UPLOADING は画面側で期限切れを判定する
export function analysisRequestStatusLabel(item: AnalysisRequestItem, now: number = Date.now()): string {
  if (item.status === 'UPLOADING') {
    return now > new Date(item.upload_expires_at).getTime() ? '期限切れ' : 'アップロード待ち'
  }
  return {
    ANALYZING: '解析中',
    SUCCEEDED: '登録完了',
    NO_DATA: '登録対象なし',
    FAILED: '解析失敗',
  }[item.status]
}
