import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { clearAuthToken } from '@/shared/auth/token'
import { jsonResponse, mockFetch, readAuthorization } from '@/test/mockFetch'
import { AppProviders } from '../providers/AppProviders'
import { routes } from './routes'

const month = new Date().toISOString().slice(0, 7)
const testUser = { user_id: 'u1', email: 'user@example.com' }
const session = { access_token: 'access-token', token_type: 'Bearer', expires_in: 900 }

// Cookie の refresh token の有無をシナリオごとに切り替える。access token は Authorization ヘッダーで判定する。
// backend.revokeSession() で「refresh token も access token も失効した」状態にできる
function mockBackend({ hasRefreshCookie }: { hasRefreshCookie: boolean }) {
  const state = { hasRefreshCookie, accessToken: 'access-token' }
  const unauthorized = () => jsonResponse({ error: 'unauthorized' }, 401)
  const isAuthorized = (init: RequestInit) => readAuthorization(init) === `Bearer ${state.accessToken}`
  const mocked = mockFetch({
    '/api/auth/refresh': () => (state.hasRefreshCookie ? jsonResponse(session) : unauthorized()),
    '/api/auth/check': ({ init }) => (isAuthorized(init) ? jsonResponse({ user: testUser }) : unauthorized()),
    '/api/auth/login': ({ init }) => {
      const body = JSON.parse(String(init.body)) as { email: string; password: string }
      return body.password === 'secret' ? jsonResponse({ ...session, user: testUser }) : unauthorized()
    },
    '/api/auth/logout': () => new Response(null, { status: 204 }),
    [`/api/months/${month}/analysis-requests`]: ({ init }) =>
      isAuthorized(init) ? jsonResponse({ items: [] }) : unauthorized(),
    '/api/monthly-summaries': ({ init }) => isAuthorized(init) ? jsonResponse({ monthly_summaries: [] }) : unauthorized(),
    [`/api/months/${month}/expenses`]: ({ init }) => isAuthorized(init) ? jsonResponse({ year_month: month, items: [] }) : unauthorized(),
  })
  return {
    ...mocked,
    revokeSession() {
      state.hasRefreshCookie = false
      state.accessToken = 'revoked'
    },
  }
}

function renderAt(path: string) {
  const router = createMemoryRouter(routes, { initialEntries: [path] })
  render(
    <AppProviders>
      <RouterProvider router={router} />
    </AppProviders>,
  )
  return router
}

describe('routes', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    vi.stubGlobal('scrollTo', vi.fn())
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    clearAuthToken()
  })

  it('セッションを復元できないときは / からログイン画面へ送る', async () => {
    mockBackend({ hasRefreshCookie: false })

    const router = renderAt('/')

    // 確認中もヘッダーは表示したまま、本文だけを差し替える。ログアウトは未確定なので出さない
    expect(screen.getByRole('status', { name: 'ログイン状態を確認しています' })).toBeInTheDocument()
    expect(screen.getByRole('banner')).toHaveTextContent('snap_kakeibo')
    expect(screen.queryByRole('button', { name: 'ログアウト' })).not.toBeInTheDocument()
    expect(await screen.findByRole('heading', { level: 1, name: 'ログイン' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/login')
    expect(screen.queryByRole('button', { name: 'ログアウト' })).not.toBeInTheDocument()
  })

  it('Cookie からセッションを復元できたときはダッシュボードを表示する', async () => {
    const { calls } = mockBackend({ hasRefreshCookie: true })

    const router = renderAt('/')

    expect(await screen.findByRole('heading', { level: 1, name: 'ダッシュボード' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'snap_kakeibo' })).toHaveAttribute('href', '/')
    expect(screen.queryByText('user@example.com')).not.toBeInTheDocument()
    expect(screen.getByRole('contentinfo')).toHaveTextContent('snap_kakeibo')
    expect(router.state.location.pathname).toBe('/')
    // メモリに token が無いので refresh → check の 2 リクエストで復元する
    expect(calls.slice(0, 2).map((call) => call.url)).toEqual(['/api/auth/refresh', '/api/auth/check'])
  })

  it('利用中にセッションが切れたら、画面の API の 401 を受けてログイン画面へ送る', async () => {
    const backend = mockBackend({ hasRefreshCookie: true })
    const router = renderAt('/analysis-requests')
    await screen.findByRole('heading', { name: '解析履歴' })

    backend.revokeSession()
    // 一覧の再読み込みが 401 → refresh も 401 になる
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    await user.click(screen.getByRole('button', { name: '再読み込み' }))

    expect(await screen.findByRole('heading', { name: 'ログイン' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/login')
  })

  it('ログイン済みで /login を開いたら / へ戻す', async () => {
    mockBackend({ hasRefreshCookie: true })

    const router = renderAt('/login')

    expect(await screen.findByRole('button', { name: 'ログアウト' })).toBeInTheDocument()
    await waitFor(() => expect(router.state.location.pathname).toBe('/'))
  })

  it('ログイン済みなら月別支出の直接URLを表示する', async () => {
    mockBackend({ hasRefreshCookie: true })
    const router = renderAt(`/months/${month}`)
    expect(await screen.findByRole('heading', { level: 1, name: '月別支出' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'この月の支出はまだありません' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe(`/months/${month}`)
  })

  it('ログインすると元の URL へ戻り、ログアウトするとログイン画面へ戻る', async () => {
    mockBackend({ hasRefreshCookie: false })
    const router = renderAt('/?tab=recent')
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

    await screen.findByRole('heading', { name: 'ログイン' })
    await user.type(screen.getByLabelText('メールアドレス'), 'user@example.com')
    await user.type(screen.getByLabelText('パスワード'), 'secret')
    await user.click(screen.getByRole('button', { name: 'ログイン' }))

    expect(await screen.findByRole('button', { name: 'ログアウト' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/')
    expect(router.state.location.search).toBe('?tab=recent')

    await user.click(screen.getByRole('button', { name: 'ログアウト' }))

    expect(await screen.findByRole('heading', { name: 'ログイン' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/login')
  })

  it('未定義の URL は / へ戻す', async () => {
    mockBackend({ hasRefreshCookie: true })

    const router = renderAt('/no-such-page')

    expect(await screen.findByRole('button', { name: 'ログアウト' })).toBeInTheDocument()
    await waitFor(() => expect(router.state.location.pathname).toBe('/'))
  })

  it('パスワードを間違えるとログイン画面に留まり、誤りを案内する', async () => {
    mockBackend({ hasRefreshCookie: false })
    const router = renderAt('/login')
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

    await screen.findByRole('heading', { name: 'ログイン' })
    await user.type(screen.getByLabelText('メールアドレス'), 'user@example.com')
    await user.type(screen.getByLabelText('パスワード'), 'wrong')
    await user.click(screen.getByRole('button', { name: 'ログイン' }))

    expect(await screen.findByRole('alert')).toHaveTextContent('メールアドレスまたはパスワードが正しくありません。')
    expect(router.state.location.pathname).toBe('/login')
  })
})
