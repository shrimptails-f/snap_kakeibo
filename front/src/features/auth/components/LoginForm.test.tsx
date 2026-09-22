import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ApiError } from '@/shared/api/client'
import { LoginForm } from './LoginForm'

describe('LoginForm', () => {
  it('入力したメールアドレスとパスワードで onSubmit を呼ぶ', async () => {
    const onSubmit = vi.fn(async () => undefined)
    render(<LoginForm onSubmit={onSubmit} />)
    const user = userEvent.setup()

    await user.type(screen.getByLabelText('メールアドレス'), 'user@example.com')
    await user.type(screen.getByLabelText('パスワード'), 'secret')
    await user.click(screen.getByRole('button', { name: 'ログイン' }))

    await waitFor(() => expect(onSubmit).toHaveBeenCalledWith({ email: 'user@example.com', password: 'secret' }))
    expect(screen.getByLabelText('パスワード')).toHaveValue('')
  })

  it('未入力のまま送信すると項目ごとの案内を出し、onSubmit を呼ばない', async () => {
    const onSubmit = vi.fn(async () => undefined)
    render(<LoginForm onSubmit={onSubmit} />)
    const user = userEvent.setup()

    await user.click(screen.getByRole('button', { name: 'ログイン' }))

    expect(await screen.findByText('メールアドレスの形式で入力してください。')).toBeInTheDocument()
    expect(screen.getByText('パスワードを入力してください。')).toBeInTheDocument()
    expect(screen.getByLabelText('メールアドレス')).toHaveAccessibleDescription('メールアドレスの形式で入力してください。')
    expect(screen.getByLabelText('メールアドレス')).toBeInvalid()
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('メールアドレスの形式が違えば送信しない', async () => {
    const onSubmit = vi.fn(async () => undefined)
    render(<LoginForm onSubmit={onSubmit} />)
    const user = userEvent.setup()

    await user.type(screen.getByLabelText('メールアドレス'), 'not-an-email')
    await user.type(screen.getByLabelText('パスワード'), 'secret')
    await user.click(screen.getByRole('button', { name: 'ログイン' }))

    expect(await screen.findByText('メールアドレスの形式で入力してください。')).toBeInTheDocument()
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('401 のときはメールアドレスまたはパスワードの誤りとして案内する', async () => {
    const onSubmit = vi.fn(async () => {
      throw new ApiError({ status: 401, apiMessage: 'invalid email or password' })
    })
    render(<LoginForm onSubmit={onSubmit} />)
    const user = userEvent.setup()

    await user.type(screen.getByLabelText('メールアドレス'), 'user@example.com')
    await user.type(screen.getByLabelText('パスワード'), 'wrong')
    await user.click(screen.getByRole('button', { name: 'ログイン' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('メールアドレスまたはパスワードが正しくありません。')
    expect(screen.getByRole('button', { name: 'ログイン' })).toBeEnabled()
  })

  it('429 のときは時間をおくよう案内する', async () => {
    const onSubmit = vi.fn(async () => {
      throw new ApiError({ status: 429, apiMessage: 'too many login attempts' })
    })
    render(<LoginForm onSubmit={onSubmit} />)
    const user = userEvent.setup()

    await user.type(screen.getByLabelText('メールアドレス'), 'user@example.com')
    await user.type(screen.getByLabelText('パスワード'), 'secret')
    await user.click(screen.getByRole('button', { name: 'ログイン' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('リクエストが集中しています。')
  })
})
