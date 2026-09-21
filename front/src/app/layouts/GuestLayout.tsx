import { Outlet } from 'react-router'
import { APP_NAME } from '@/shared/config/app'

// ログイン前(/login など)の画面共通の枠。ナビや利用者情報は出さない
export function GuestLayout() {
  return (
    <div className="appLayout">
      <header className="appHeader">
        <div className="page-shell appHeader__inner">
          <span className="appHeader__brand">{APP_NAME}</span>
        </div>
      </header>
      <main className="page-shell appLayout__main">
        <Outlet />
      </main>
    </div>
  )
}
