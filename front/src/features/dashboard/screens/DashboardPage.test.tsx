import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
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
    expect(screen.getByText('表示期間：2026年4月〜2026年9月')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'カテゴリ別（明細）' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('list', { name: '月ごとのカテゴリ別明細合計' })).toBeInTheDocument()
    expect(screen.getByText('明細合計').parentElement).toHaveTextContent(/[¥￥]140,500/)
    expect(screen.getByText('対象月の計上額').parentElement).toHaveTextContent(/[¥￥]128,500/)
    expect(screen.getByText('前月からの変化').parentElement).toHaveTextContent(/\+[¥￥]18,500/)
    expect(screen.getByText(/集計更新：2026年9月15日 21:01/)).toBeInTheDocument()
  })

  it('カテゴリ区画に詳細パネルを出さず、参考月は選択欄で切り替える', async () => {
    mockFetch({ '/api/monthly-summaries': summaries })
    const router = renderPage()
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

    await screen.findByRole('list', { name: '月ごとのカテゴリ別明細合計' })
    expect(screen.queryByRole('complementary')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /内訳を表示/ })).not.toBeInTheDocument()

    await user.selectOptions(screen.getByLabelText('対象月'), '2026-08')
    expect(screen.getByLabelText('対象月')).toHaveValue('2026-08')
    expect(router.state.location.search).toContain('reference=2026-08')
  })

  it('計上額表示では棒から月別支出へ移動できる', async () => {
    mockFetch({ '/api/monthly-summaries': summaries })
    const router = renderPage()
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

    await screen.findByRole('heading', { name: '月ごとの支出' })
    await user.click(screen.getByRole('button', { name: '計上額' }))
    const monthLink = screen.getByRole('link', { name: /2026年9月、計上額.*月別支出を見る/ })
    await user.click(monthLink)
    expect(router.state.location.pathname).toBe('/months/2026-09')
  })

  it('前の6か月へ移動し、負数と集計のない月を区別する', async () => {
    mockFetch({ '/api/monthly-summaries': summaries })
    const router = renderPage()
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

    await user.click(await screen.findByRole('button', { name: '‹ 前の6か月' }))
    expect(screen.getByText('表示期間：2025年10月〜2026年3月')).toBeInTheDocument()
    expect(screen.getAllByText(/-[¥￥]1,000/).length).toBeGreaterThan(0)
    expect(screen.getAllByText('—（集計なし）')).toHaveLength(5)
    expect(router.state.location.search).toContain('end=2026-03')
    expect(screen.getByRole('button', { name: '‹ 前の6か月' })).toBeDisabled()
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
  })
})
