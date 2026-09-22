import { isApiError } from '@/shared/api/client'
import { toFriendlyMessage } from '@/shared/api/errors'

// ログイン画面向けの文言。401 は「有効期限切れ」ではなく入力誤りとして案内する
export function loginErrorMessage(error: unknown): string {
  if (isApiError(error) && error.status === 401) {
    return 'メールアドレスまたはパスワードが正しくありません。'
  }
  return toFriendlyMessage(error)
}
