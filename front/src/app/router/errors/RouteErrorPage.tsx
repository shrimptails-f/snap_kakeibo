import { useRouteError } from 'react-router'
import { toFriendlyMessage } from '@/shared/api/errors'
import { Button } from '@/shared/ui/Button'
import styles from './RouteErrorPage.module.css'

// 画面の描画中に処理できなかった失敗(useSuspenseQuery の初回取得失敗など)を受ける。
// レイアウトとガードは残るので、セッション切れの場合は AuthGuard がログイン画面へ送る
export function RouteErrorPage() {
  const error = useRouteError()

  return (
    <section className={styles.page}>
      <h1>表示できませんでした</h1>
      <p role="alert">{toFriendlyMessage(error)}</p>
      <Button variant="secondary" onClick={() => window.location.reload()}>
        再読み込み
      </Button>
    </section>
  )
}
