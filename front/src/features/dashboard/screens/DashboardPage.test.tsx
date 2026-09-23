import { QueryClientProvider } from '@tanstack/react-query'
import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/app/providers/queryClient'
import { clearAuthToken, setAuthSession } from '@/shared/auth/token'
import { jsonResponse, mockFetch } from '@/test/mockFetch'
import { DashboardPage } from './DashboardPage'

const zeroCategories = {
  food: 0, daily_goods: 0, medical: 0, transport: 0, utilities: 0, entertainment: 0,
  social: 0, clothing: 0, education: 0, other: 0, unknown: 0,
}

const summaries = {
  monthly_summaries: [
    {
      year_month: '2026-09', total_recorded_amount: 128500, expense_count: 25, detail_count: 120,
      category_totals: { ...zeroCategories, food: 86000, daily_goods: 22500, social: 15000, other: 5000, unknown: 12000 },
      updated_at: '2026-09-15T12:01:00Z',
    },
    {
      year_month: '2026-08', total_recorded_amount: 110000, expense_count: 20, detail_count: 80,
      category_totals: { ...zeroCategories, food: 70000, daily_goods: 40000 },
      updated_at: '2026-08-31T12:00:00Z',
    },
    {
      year_month: '2026-03', total_recorded_amount: -1000, expense_count: 1, detail_count: 1,
      category_totals: { ...zeroCategories, other: -1000 }, updated_at: '2026-03-20T00:00:00Z',
    },
  ],
}

function renderPage(path = '/') {
  const router = createMemoryRouter([
    { path: '/', element: <DashboardPage /> },
    { path: '/months/:yearMonth', element: <h1>月別支出</h1> },
    { path: '/upload', element: <h1>取り込み</h1> },
    { path: '/analysis-requests', element: <h1>解析履歴</h1> },
  ], { initialEntries: [path] })
  render(<QueryClientProvider client={createQueryClient()}><RouterProvider router={router} /></QueryClientProvider>)
  return router
}

describe('DashboardPage', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    vi.setSystemTime(new Date('2026-09-22T03:00:00+09:00'))
    setAuthSession({ access_token: 'token', token_type: 'Bearer', expires_in: 900 })
    window.sessionStorage.clear()
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    clearAuthToken()
  })

  it('直近6か月をカテゴリ別で表示し、計上額との差と前月比を明示する', async () => {
    mockFetch({ '/api/monthly-summaries': summaries })
    renderPage()

    expect(screen.getByRole('status', { name: '月ごとの支出を読み込み中' })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { level: 1, name: 'ダッシュボード' })).toBeInTheDocument()
    expect(screen.getByRole('combobox', { name: '表示開始月' })).toHaveValue('2026-04')
    expect(screen.getByRole('combobox', { name: '表示終了月' })).toHaveValue('2026-09')
    expect(screen.queryByText(/表示期間：/)).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'カテゴリ別（明細）' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('list', { name: '月ごとのカテゴリ別明細合計' })).toBeInTheDocument()
    expect(screen.getByText('明細合計').parentElement).toHaveTextContent(/[¥￥]140,500/)
    expect(screen.getByText('対象月の計上額').parentElement).toHaveTextContent(/[¥￥]128,500/)
    expect(screen.getByText('前月からの変化').parentElement).toHaveTextContent(/\+[¥￥]18,500/)
    expect(screen.queryByText(/集計更新：/)).not.toBeInTheDocument()
  })

  it('積み上げ棒を選ぶとカテゴリ別内訳の月が切り替わる', async () => {
    mockFetch({ '/api/monthly-summaries': summaries })
    const router = renderPage()
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

    await screen.findByRole('list', { name: '月ごとのカテゴリ別明細合計' })
    expect(screen.queryByRole('complementary')).not.toBeInTheDocument()
    expect(screen.queryByRole('combobox', { name: '対象月' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '2026年8月のカテゴリ別内訳を表示' }))
    expect(screen.getByRole('button', { name: '2026年8月のカテゴリ別内訳を表示' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('link', { name: '2026年8月の明細を見る ›' })).toHaveAttribute('href', '/months/2026-08')
    expect(router.state.location.search).toContain('reference=2026-08')
  })

  it('計上額表示の棒でもカテゴリ別内訳の月だけが切り替わる', async () => {
    mockFetch({ '/api/monthly-summaries': summaries })
    const router = renderPage()
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

    await screen.findByRole('heading', { name: '月ごとの支出' })
    await user.click(screen.getByRole('button', { name: '計上額' }))
    const monthButton = screen.getByRole('button', { name: /2026年8月のカテゴリ別内訳を表示（計上額/ })
    await user.click(monthButton)
    expect(router.state.location.pathname).toBe('/')
    expect(monthButton).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('link', { name: '2026年8月の明細を見る ›' })).toHaveAttribute('href', '/months/2026-08')
  })

  it('開始月または終了月を変えると6か月幅を保ち、負数と集計なしを区別する', async () => {
    mockFetch({ '/api/monthly-summaries': summaries })
    const router = renderPage()
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

    await screen.findByRole('combobox', { name: '表示開始月' })
    await user.selectOptions(screen.getByRole('combobox', { name: '表示開始月' }), '2025-10')
    expect(screen.getByRole('combobox', { name: '表示終了月' })).toHaveValue('2026-03')
    expect(screen.getAllByText(/-[¥￥]1,000/).length).toBeGreaterThan(0)
    expect(screen.getAllByText('集計なし')).toHaveLength(10)
    expect(router.state.location.search).toContain('end=2026-03')
    await user.selectOptions(screen.getByRole('combobox', { name: '表示終了月' }), '2026-08')
    expect(screen.getByRole('combobox', { name: '表示開始月' })).toHaveValue('2026-03')
  })

  it('全件空なら取り込みと解析履歴への回復導線だけを表示する', async () => {
    mockFetch({ '/api/monthly-summaries': { monthly_summaries: [] } })
    renderPage()

    expect(await screen.findByRole('heading', { name: 'まだ月ごとの集計がありません' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /レシートを取り込む/ })).toHaveAttribute('href', '/upload')
    expect(screen.getByRole('link', { name: /解析履歴を確認/ })).toHaveAttribute('href', '/analysis-requests')
    expect(screen.queryByRole('list', { name: /月ごと/ })).not.toBeInTheDocument()
  })

  it('初回取得失敗から再試行できる', async () => {
    let attempts = 0
    mockFetch({ '/api/monthly-summaries': () => ++attempts === 1 ? jsonResponse({ error: 'boom' }, 500) : jsonResponse({ monthly_summaries: [] }) })
    renderPage()
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('月ごとの支出を取得できませんでした')
    await user.click(within(alert).getByRole('button', { name: '再試行' }))
    expect(await screen.findByRole('heading', { name: 'まだ月ごとの集計がありません' })).toBeInTheDocument()
  })

  it('再取得に失敗しても表示中の集計を維持する', async () => {
    let attempts = 0
    mockFetch({ '/api/monthly-summaries': () => ++attempts === 1 ? jsonResponse(summaries) : jsonResponse({ error: 'boom' }, 500) })
    renderPage()
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

    await screen.findByText('対象月の計上額')
    await user.click(screen.getByRole('button', { name: '再読み込み' }))
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('前回取得した内容を表示しています'))
    expect(screen.getByText('対象月の計上額').parentElement).toHaveTextContent(/[¥￥]128,500/)
    expect(screen.getByRole('button', { name: '再読み込み（あと5秒）' })).toBeDisabled()
    act(() => vi.advanceTimersByTime(2100))
    expect(screen.getByRole('button', { name: '再読み込み（あと3秒）' })).toBeDisabled()
    act(() => vi.advanceTimersByTime(2900))
    expect(screen.getByRole('button', { name: '再読み込み' })).toBeEnabled()
  })
})
