import { APP_NAME } from '@/shared/config/app'
import styles from './AppFooter.module.css'

// 載せるものが決まるまでアプリ名だけの最小構成にする
export function AppFooter() {
  return (
    <footer className={styles.footer}>
      <div className="page-shell">
        <p>{APP_NAME}</p>
      </div>
    </footer>
  )
}
