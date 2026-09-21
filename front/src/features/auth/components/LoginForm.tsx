import { useState } from 'react'
import type { FormEvent } from 'react'
import { loginErrorMessage } from '../lib/loginErrorMessage'
import type { LoginRequest } from '../types/auth.types'

type Props = {
  onSubmit: (request: LoginRequest) => Promise<void>
}

export function LoginForm({ onSubmit }: Props) {
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setIsSubmitting(true)
    setError(null)
    try {
      await onSubmit({ email, password })
      setPassword('')
    } catch (e: unknown) {
      setError(loginErrorMessage(e))
    } finally {
      setIsSubmitting(false)
    }
  }

  return (
    <form className="loginForm" onSubmit={handleSubmit}>
      <div className="loginForm__field">
        <label htmlFor="login-email">メールアドレス</label>
        <input
          id="login-email"
          autoComplete="email"
          inputMode="email"
          onChange={(e) => setEmail(e.target.value)}
          required
          type="email"
          value={email}
        />
      </div>
      <div className="loginForm__field">
        <label htmlFor="login-password">パスワード</label>
        <input
          id="login-password"
          autoComplete="current-password"
          onChange={(e) => setPassword(e.target.value)}
          required
          type="password"
          value={password}
        />
      </div>
      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
      <button disabled={isSubmitting} type="submit">
        {isSubmitting ? 'ログイン中...' : 'ログイン'}
      </button>
    </form>
  )
}
