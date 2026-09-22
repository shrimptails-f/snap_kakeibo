import { useState } from 'react'
import { Link, NavLink, useLocation } from 'react-router'
import { useAuthSession } from '@/features/auth'
import { APP_NAME } from '@/shared/config/app'
import { Button } from '@/shared/ui/Button'
import styles from './AppHeader.module.css'

// ログイン後の全画面に共通するヘッダー。
export function AppHeader() {
  const { isAuthorized, logout } = useAuthSession()
  const location = useLocation()
  const [isLoggingOut, setIsLoggingOut] = useState(false)
  const now = new Date()
  const currentMonth = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
  const isMonthPage = location.pathname.startsWith('/months/')

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
        {isAuthorized && (
          <nav className={styles.navigation} aria-label="メインナビゲーション">
            <NavLink to="/" end className={({ isActive }) => isActive ? styles.activeNavLink : styles.navLink}>ダッシュボード</NavLink>
            <Link to={`/months/${currentMonth}`} aria-current={isMonthPage ? 'page' : undefined} className={isMonthPage ? styles.activeNavLink : styles.navLink}>月別支出</Link>
            <NavLink to="/upload" className={({ isActive }) => isActive ? styles.activeNavLink : styles.navLink}>取り込む</NavLink>
            <NavLink to="/analysis-requests" className={({ isActive }) => isActive ? styles.activeNavLink : styles.navLink}>解析履歴</NavLink>
          </nav>
        )}
        {/* セッション確認中はログイン状態が未確定なので、ログアウトを出さない */}
        {isAuthorized && (
          <div className={styles.sessionBar}>
            <Button
              variant="secondary"
              className={styles.logoutButton}
              onClick={() => void handleLogout()}
              disabled={isLoggingOut}
            >
              {isLoggingOut ? 'ログアウト中...' : 'ログアウト'}
            </Button>
          </div>
        )}
      </div>
    </header>
  )
}
