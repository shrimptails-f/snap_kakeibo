// セッション復元中の本文。レイアウト(ヘッダー・フッター)は残し、コンテンツ部分だけを差し替える
export function SessionCheckingNotice() {
  return (
    <p role="status" className="muted">
      ログイン状態を確認しています...
    </p>
  )
}
