import { LoginForm } from '../components/LoginForm'
import { useAuthSession } from '../hooks/useAuthSession'
import styles from './LoginPage.module.css'

// ログイン成功後の遷移は GuestGuard が行う(ログイン済みになった時点で元の URL か / へ戻す)
export function LoginPage() {
  const { login } = useAuthSession()

  return (
    <section className={styles.panel}>
      <h1 className={styles.title}>ログイン</h1>
      <LoginForm onSubmit={login} />
    </section>
  )
}
