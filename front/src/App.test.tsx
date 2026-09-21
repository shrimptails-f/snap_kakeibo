import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import App from './App.tsx'

// 通貨の記号(¥ / ￥)は Node の ICU によって変わるので、画面と同じフォーマッタで期待値を作る
function yen(value: number) {
  return new Intl.NumberFormat('ja-JP', { style: 'currency', currency: 'JPY' }).format(value)
}

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

  it('解析依頼が空のときは案内を表示する', async () => {
    const month = new Date().toISOString().slice(0, 7)
    const fetchMock = mockFetch({ [`/api/months/${month}/analysis-requests`]: { items: [] } })

    render(<App />)

    expect(screen.getByRole('heading', { level: 1, name: 'snap_kakeibo' })).toBeInTheDocument()
    expect(await screen.findByText('まだ解析依頼がありません。')).toBeInTheDocument()
    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith(`/api/months/${month}/analysis-requests`, expect.any(Object)))
  })

  it('解析依頼を一覧に表示し、登録完了していない行は選択できない', async () => {
    const month = new Date().toISOString().slice(0, 7)
    mockFetch({
      [`/api/months/${month}/analysis-requests`]: {
        items: [
          {
            analysis_request_id: 'req1',
            expense_id: 'e1',
            status: 'SUCCEEDED',
            attempt: 1,
            file_name: 'receipt-1.jpg',
            year_month: month,
            upload_expires_at: '2026-09-18T00:15:00Z',
            created_at: '2026-09-18T00:00:00Z',
            updated_at: '2026-09-18T00:00:00Z',
          },
          {
            analysis_request_id: 'req2',
            status: 'ANALYZING',
            attempt: 2,
            file_name: 'receipt-2.jpg',
            year_month: month,
            upload_expires_at: '2026-09-18T00:15:00Z',
            created_at: '2026-09-18T00:00:00Z',
            updated_at: '2026-09-18T00:00:00Z',
          },
          {
            analysis_request_id: 'req3',
            status: 'UPLOADING',
            attempt: 1,
            file_name: 'receipt-3.jpg',
            year_month: month,
            upload_expires_at: '2020-01-01T00:00:00Z',
            created_at: '2020-01-01T00:00:00Z',
            updated_at: '2020-01-01T00:00:00Z',
          },
        ],
      },
    })

    render(<App />)

    const done = await screen.findByRole('button', { name: /receipt-1\.jpg/ })
    const analyzing = screen.getByRole('button', { name: /receipt-2\.jpg/ })
    expect(done).toBeEnabled()
    expect(analyzing).toBeDisabled()
    expect(screen.getByText('期限切れ')).toBeInTheDocument()
    expect(screen.getByText('試行 2 回目')).toBeInTheDocument()
  })

  it('登録完了した解析依頼を選ぶと支出と明細をカテゴリの表示名付きで表示する', async () => {
    const month = new Date().toISOString().slice(0, 7)
    mockFetch({
      [`/api/months/${month}/analysis-requests`]: {
        items: [
          {
            analysis_request_id: 'req1',
            expense_id: 'e1',
            status: 'SUCCEEDED',
            attempt: 1,
            file_name: 'receipt-1.jpg',
            year_month: month,
            upload_expires_at: '2026-09-18T00:15:00Z',
            created_at: '2026-09-18T00:00:00Z',
            updated_at: '2026-09-18T00:00:00Z',
          },
        ],
      },
      '/api/expenses/e1': {
        expense: {
          expense_id: 'e1',
          store_name: '居酒屋',
          purchase_date: '2026-09-18',
          read_amount: 5000,
          adjustment_amount: -2500,
          recorded_amount: 2500,
          source: 'AI',
          is_edited: true,
          updated_at: '2026-09-18T01:00:00Z',
        },
        details: [{ detail_id: 'd1', name: '飲み会', category: 'social', amount: 5000, quantity: 1, source: 'AI', is_edited: true }],
      },
    })

    render(<App />)

    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    await user.click(await screen.findByRole('button', { name: /receipt-1\.jpg/ }))

    expect(await screen.findByText('居酒屋')).toBeInTheDocument()
    expect(screen.getByText(yen(2500))).toBeInTheDocument()
    expect(screen.getByText(`読取金額 ${yen(5000)} / 調整額 ${yen(-2500)}`)).toBeInTheDocument()
    expect(screen.getByText('交際・会食')).toBeInTheDocument()
    expect(screen.getByText(/AI由来・手動編集済み/)).toBeInTheDocument()
    expect(screen.getByText('（手動編集済み）')).toBeInTheDocument()
  })

  it('取得に失敗したらエラーを表示する', async () => {
    mockFetch({})

    render(<App />)

    expect(await screen.findByText('Error: 404 Not Found')).toBeInTheDocument()
  })
})
