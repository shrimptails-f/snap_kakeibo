import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { loginErrorMessage } from '../lib/loginErrorMessage'
import { loginFormSchema } from '../types/login.schema'
import type { LoginFormValues } from '../types/login.schema'

type Props = {
  onSubmit: (values: LoginFormValues) => Promise<void>
}

// 入力の検証は login.schema.ts(zod)、送信中・エラーの状態は react-hook-form が持つ。
// ブラウザ標準の検証(noValidate)は使わず、文言を利用者向けに揃える
export function LoginForm({ onSubmit }: Props) {
  const {
    register,
    handleSubmit,
    setError,
    resetField,
    formState: { errors, isSubmitting },
  } = useForm<LoginFormValues>({
    resolver: zodResolver(loginFormSchema),
    defaultValues: { email: '', password: '' },
  })

  async function submit(values: LoginFormValues) {
    try {
      await onSubmit(values)
      resetField('password')
    } catch (e: unknown) {
      // サーバー由来の失敗はフィールドではなくフォーム全体のエラーにする。次の送信で消える
      setError('root.server', { message: loginErrorMessage(e) })
    }
  }

  return (
    <form className="loginForm" noValidate onSubmit={handleSubmit(submit)}>
      <div className="loginForm__field">
        <label htmlFor="login-email">メールアドレス</label>
        <input
          id="login-email"
          autoComplete="email"
          inputMode="email"
          required
          type="email"
          aria-invalid={errors.email ? true : undefined}
          aria-describedby={errors.email ? 'login-email-error' : undefined}
          {...register('email')}
        />
        {errors.email && (
          <p id="login-email-error" className="loginForm__fieldError">
            {errors.email.message}
          </p>
        )}
      </div>
      <div className="loginForm__field">
        <label htmlFor="login-password">パスワード</label>
        <input
          id="login-password"
          autoComplete="current-password"
          required
          type="password"
          aria-invalid={errors.password ? true : undefined}
          aria-describedby={errors.password ? 'login-password-error' : undefined}
          {...register('password')}
        />
        {errors.password && (
          <p id="login-password-error" className="loginForm__fieldError">
            {errors.password.message}
          </p>
        )}
      </div>
      {errors.root?.server && (
        <p className="error" role="alert">
          {errors.root.server.message}
        </p>
      )}
      <button disabled={isSubmitting} type="submit">
        {isSubmitting ? 'ログイン中...' : 'ログイン'}
      </button>
    </form>
  )
}
