import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App.tsx'

// fetch を差し替えて、パスごとの応答を返す
function mockFetch(routes: Record<string, unknown>) {
  const defaultRoutes: Record<string, unknown> = {
    '/api/auth/refresh': {
      access_token: 'test-token',
      token_type: 'Bearer',
      expires_in: 3600,
    },
    '/api/auth/check': {
      user: { user_id: 'test-user', email: 'test@example.com' },
    },
  }
  const mergedRoutes = { ...defaultRoutes, ...routes }
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
    const path = Object.keys(mergedRoutes).find((p) => url.endsWith(p))
    if (path === undefined) {
      return new Response(null, { status: 404, statusText: 'Not Found' })
    }
    return Response.json(mergedRoutes[path])
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

describe('App', () => {
  beforeEach(() => {
    // 5 秒ごとの再取得タイマーがテスト終了後に走らないようにする
    vi.useFakeTimers({ shouldAdvanceTime: true })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('履歴が空のときは案内を表示する', async () => {
    const month = new Date().toISOString().slice(0, 7)
    const fetchMock = mockFetch({ [`/api/months/${month}/uploads`]: { items: [] } })

    render(<App />)

    expect(screen.getByRole('heading', { level: 1, name: 'snap_kakeibo' })).toBeInTheDocument()
    expect(await screen.findByText('まだ履歴がありません。')).toBeInTheDocument()
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(`/api/months/${month}/uploads`, expect.any(Object)))
  })

  it('履歴を一覧に表示し、解析完了していない行は選択できない', async () => {
    const month = new Date().toISOString().slice(0, 7)
    mockFetch({
      [`/api/months/${month}/uploads`]: {
        items: [
          {
            upload_id: 'up1',
            billing_id: 'b1',
            status: 'SUCCEEDED',
            file_name: 'receipt-1.jpg',
            year_month: month,
            created_at: '2026-09-18T00:00:00Z',
            updated_at: '2026-09-18T00:00:00Z',
          },
          {
            upload_id: 'up2',
            status: 'ANALYZING',
            file_name: 'receipt-2.jpg',
            year_month: month,
            created_at: '2026-09-18T00:00:00Z',
            updated_at: '2026-09-18T00:00:00Z',
          },
        ],
      },
    })

    render(<App />)

    const done = await screen.findByRole('button', { name: /receipt-1\.jpg/ })
    const analyzing = screen.getByRole('button', { name: /receipt-2\.jpg/ })
    expect(done).toBeEnabled()
    expect(analyzing).toBeDisabled()
  })

  it('取得に失敗したらエラーを表示する', async () => {
    mockFetch({})

    render(<App />)

    expect(await screen.findByText('Error: 404 Not Found')).toBeInTheDocument()
  })
})
