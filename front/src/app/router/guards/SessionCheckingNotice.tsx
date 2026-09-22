import { SpinnerBlock } from '@/shared/ui/Spinner'

// セッション復元中の本文。レイアウト(ヘッダー・フッター)は残し、コンテンツ部分だけを差し替える
export function SessionCheckingNotice() {
  return <SpinnerBlock label="ログイン状態を確認しています" />
}
