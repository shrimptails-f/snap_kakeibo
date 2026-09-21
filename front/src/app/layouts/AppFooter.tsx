import { APP_NAME } from '@/shared/config/app'

// 載せるものが決まるまでアプリ名だけの最小構成にする
export function AppFooter() {
  return (
    <footer className="appFooter">
      <div className="page-shell">
        <p>{APP_NAME}</p>
      </div>
    </footer>
  )
}
