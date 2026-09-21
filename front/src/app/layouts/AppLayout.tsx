import { Outlet } from 'react-router'
import { useAuthSession } from '@/features/auth'

// ログイン後の全画面に共通するヘッダー(アプリ名、ログイン中の利用者、ログアウト)
export function AppLayout() {
  const { user, logout } = useAuthSession()

  return (
    <main className="page-shell">
      <header className="appHeader">
        <h1>snap_kakeibo</h1>
        <div className="sessionBar">
          <span>{user?.email}</span>
          <button type="button" onClick={() => void logout()}>
            ログアウト
          </button>
        </div>
      </header>
      <Outlet />
    </main>
  )
}
