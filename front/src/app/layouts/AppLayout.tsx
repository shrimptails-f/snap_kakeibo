import { Outlet } from 'react-router'
import { AppFooter } from './AppFooter'
import { AppHeader } from './AppHeader'

// ログイン後の画面共通の枠。各画面は本文だけを描く
export function AppLayout() {
  return (
    <div className="appLayout">
      <AppHeader />
      <main className="page-shell appLayout__main">
        <Outlet />
      </main>
      <AppFooter />
    </div>
  )
}
