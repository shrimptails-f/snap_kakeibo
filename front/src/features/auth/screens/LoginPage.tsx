import { LoginForm } from '../components/LoginForm'
import { useAuthSession } from '../hooks/useAuthSession'

// ログイン成功後の遷移は GuestGuard が行う(ログイン済みになった時点で元の URL か / へ戻す)
export function LoginPage() {
  const { login } = useAuthSession()

  return (
    <main>
      <section className="loginPanel">
        <h1>snap_kakeibo</h1>
        <h2>ログイン</h2>
        <LoginForm onSubmit={login} />
      </section>
    </main>
  )
}
