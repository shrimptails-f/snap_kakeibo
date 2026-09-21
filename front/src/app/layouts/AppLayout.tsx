import { Suspense } from 'react'
import { Outlet } from 'react-router'
import { SpinnerBlock } from '@/shared/ui/Spinner'
import { AppFooter } from './AppFooter'
import { AppHeader } from './AppHeader'

// ログイン後の画面共通の枠。各画面は本文だけを描く。
// 画面の初回 loading(useSuspenseQuery)はここの Suspense が受け、画面側でスピナーを書かない
export function AppLayout() {
  return (
    <div className="appLayout">
      <AppHeader />
      <main className="page-shell appLayout__main">
        <Suspense fallback={<SpinnerBlock label="画面を読み込んでいます" />}>
          <Outlet />
        </Suspense>
      </main>
      <AppFooter />
    </div>
  )
}
