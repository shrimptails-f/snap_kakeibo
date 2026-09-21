import { Suspense } from 'react'
import { Outlet } from 'react-router'
import { APP_NAME } from '@/shared/config/app'
import { SpinnerBlock } from '@/shared/ui/Spinner'
import headerStyles from './AppHeader.module.css'
import styles from './AppLayout.module.css'

// ログイン前(/login など)の画面共通の枠。ナビや利用者情報は出さない
export function GuestLayout() {
  return (
    <div className={styles.layout}>
      <header className={headerStyles.header}>
        <div className={`page-shell ${headerStyles.inner}`}>
          <span className={headerStyles.brand}>{APP_NAME}</span>
        </div>
      </header>
      <main className={`page-shell ${styles.main}`}>
        <Suspense fallback={<SpinnerBlock label="画面を読み込んでいます" />}>
          <Outlet />
        </Suspense>
      </main>
    </div>
  )
}
