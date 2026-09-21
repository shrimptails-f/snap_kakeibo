import { LoginForm } from '../components/LoginForm'
import { useAuthSession } from '../hooks/useAuthSession'

// ログイン成功後の遷移は GuestGuard が行う(ログイン済みになった時点で元の URL か / へ戻す)
export function LoginPage() {
  const { login } = useAuthSession()

  return (
    <section className="loginPanel">
      <h1>ログイン</h1>
      <LoginForm onSubmit={login} />
    </section>
  )
}
