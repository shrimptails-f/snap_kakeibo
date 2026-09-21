import { useState } from 'react'
import { Link, NavLink } from 'react-router'
import { useAuthSession } from '@/features/auth'
import { APP_NAME } from '@/shared/config/app'

// ログイン後の全画面に共通するヘッダー。
// ナビ項目は画面の実装時に追加する。項目が増えて収まらなくなったら Popover(headless library)を検討する
const NAV_ITEMS = [{ to: '/', label: 'ホーム' }] as const

export function AppHeader() {
  const { user, logout } = useAuthSession()
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
        <nav aria-label="メイン" className="appNav">
          {NAV_ITEMS.map((item) => (
            <NavLink key={item.to} to={item.to} end className="appNav__link">
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="sessionBar">
          <span className="sessionBar__user">{user?.email}</span>
          <button type="button" onClick={() => void handleLogout()} disabled={isLoggingOut}>
            {isLoggingOut ? 'ログアウト中...' : 'ログアウト'}
          </button>
        </div>
      </div>
    </header>
  )
}
