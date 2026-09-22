import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { loginErrorMessage } from '../lib/loginErrorMessage'
import { loginFormSchema } from '../types/login.schema'
import type { LoginFormValues } from '../types/login.schema'
import styles from './LoginForm.module.css'

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
    <form className={styles.form} noValidate onSubmit={handleSubmit(submit)}>
      <div className={styles.field}>
        <label className={styles.label} htmlFor="login-email">メールアドレス</label>
        <input
          id="login-email"
          className={styles.input}
          autoComplete="email"
          inputMode="email"
          required
          type="email"
          aria-invalid={errors.email ? true : undefined}
          aria-describedby={errors.email ? 'login-email-error' : undefined}
          {...register('email')}
        />
        {errors.email && (
          <p id="login-email-error" className={styles.fieldError}>
            {errors.email.message}
          </p>
        )}
      </div>
      <div className={styles.field}>
        <label className={styles.label} htmlFor="login-password">パスワード</label>
        <input
          id="login-password"
          className={styles.input}
          autoComplete="current-password"
          required
          type="password"
          aria-invalid={errors.password ? true : undefined}
          aria-describedby={errors.password ? 'login-password-error' : undefined}
          {...register('password')}
        />
        {errors.password && (
          <p id="login-password-error" className={styles.fieldError}>
            {errors.password.message}
          </p>
        )}
      </div>
      {errors.root?.server && (
        <p className={styles.formError} role="alert">
          {errors.root.server.message}
        </p>
      )}
      <button className={styles.submit} disabled={isSubmitting} type="submit">
        {isSubmitting ? 'ログイン中...' : 'ログイン'}
      </button>
    </form>
  )
}
