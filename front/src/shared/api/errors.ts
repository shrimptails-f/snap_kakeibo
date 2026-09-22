import { isApiError } from './client'

// 通信エラーを利用者向けの日本語文言にする。
// HTTP ステータスやバックエンドの英文メッセージは画面へ出さない。
// 画面固有の文言(ログインのメール・パスワード誤りなど)は feature 側で status を見て上書きする
export function toFriendlyMessage(error: unknown): string {
  if (isApiError(error)) {
    if (error.status === 401) return 'ログインの有効期限が切れました。再度ログインしてください。'
    if (error.status === 400) return '入力内容に誤りがあります。内容を確認して再度お試しください。'
    if (error.status === 404) return '対象のデータが見つかりません。'
    if (error.status === 429) return 'リクエストが集中しています。しばらく待ってから再度お試しください。'
    if (error.status >= 500) return 'サーバーでエラーが発生しました。時間をおいて再度お試しください。'
  }
  return '通信に失敗しました。接続を確認して再度お試しください。'
}
