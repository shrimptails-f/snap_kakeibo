import { useState } from 'react'
import { Link } from 'react-router'
import { useAuthSession } from '@/features/auth'
import { APP_NAME } from '@/shared/config/app'
import styles from './AppHeader.module.css'

// ログイン後の全画面に共通するヘッダー。アプリ名を押すとログイン後のトップページへ戻る。
// 画面が増えたらナビゲーションをここに追加する
export function AppHeader() {
  const { isAuthorized, logout } = useAuthSession()
  const [isLoggingOut, setIsLoggingOut] = useState(false)

  async function handleLogout() {
    setIsLoggingOut(true)
    try {
      await logout()
    } finally {
      setIsLoggingOut(false)
    }
  }

  return (
    <header className={styles.header}>
      <div className={`page-shell ${styles.inner}`}>
        <Link to="/" className={styles.brand}>
          {APP_NAME}
        </Link>
        {/* セッション確認中はログイン状態が未確定なので、ログアウトを出さない */}
        {isAuthorized && (
          <div className={styles.sessionBar}>
            <button
              className={styles.logoutButton}
              type="button"
              onClick={() => void handleLogout()}
              disabled={isLoggingOut}
            >
              {isLoggingOut ? 'ログアウト中...' : 'ログアウト'}
            </button>
          </div>
        )}
      </div>
    </header>
  )
}
