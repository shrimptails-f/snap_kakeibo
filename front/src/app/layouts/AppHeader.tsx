import { useState } from 'react'
import { Link } from 'react-router'
import { useAuthSession } from '@/features/auth'
import { APP_NAME } from '@/shared/config/app'

// ログイン後の全画面に共通するヘッダー。アプリ名を押すとログイン後のトップページへ戻る。
// 画面が増えたらナビゲーションをここに追加する
export function AppHeader() {
  const { logout } = useAuthSession()
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
    <header className="appHeader">
      <div className="page-shell appHeader__inner">
        <Link to="/" className="appHeader__brand">
          {APP_NAME}
        </Link>
        <div className="sessionBar">
          <button type="button" onClick={() => void handleLogout()} disabled={isLoggingOut}>
            {isLoggingOut ? 'ログアウト中...' : 'ログアウト'}
          </button>
        </div>
      </div>
    </header>
  )
}
