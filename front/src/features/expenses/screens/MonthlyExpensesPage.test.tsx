import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/app/providers/queryClient'
import { clearAuthToken, setAuthSession } from '@/shared/auth/token'
import { jsonResponse, mockFetch } from '@/test/mockFetch'
import { MonthlyExpensesPage } from './MonthlyExpensesPage'

const summaries = {
  monthly_summaries: [{
    year_month: '2026-09', total_recorded_amount: 12000, expense_count: 2, detail_count: 3,
    category_totals: { food: 7500, daily_goods: 4500, medical: 0, transport: 0, utilities: 0, entertainment: 0, social: 0, clothing: 0, education: 0, other: 0, unknown: 1500 },
    updated_at: '2026-09-22T01:00:00Z',
  }],
}
const expenses = {
  year_month: '2026-09', items: [
    { detail_id: 'd1', expense_id: 'e1', name: '米 5kg', category: 'food', amount: 7500, quantity: 1, source: 'AI', is_edited: false, store_name: 'スーパーさくら', purchase_date: '2026-09-15' },
    { detail_id: 'd2', expense_id: 'e2', name: '洗剤セット', category: 'daily_goods', amount: 4500, quantity: 1, source: 'USER', is_edited: true, store_name: '日用品ストア', purchase_date: '2026-09-12' },
    { detail_id: 'd3', expense_id: 'e1', name: '商品A', category: 'unknown', amount: 1500, quantity: 1, source: 'AI', is_edited: false, store_name: 'スーパーさくら', purchase_date: '2026-09-15' },
  ],
}

function renderPage(path = '/months/2026-09') {
  const router = createMemoryRouter([
    { path: '/months/:yearMonth', element: <MonthlyExpensesPage /> },
    { path: '/expenses/:expenseId', element: <h1>支出詳細</h1> },
    { path: '/', element: <h1>レシート取り込み</h1> },
  ], { initialEntries: [path] })
  render(<QueryClientProvider client={createQueryClient()}><RouterProvider router={router} /></QueryClientProvider>)
  return router
}

describe('MonthlyExpensesPage', () => {
  beforeEach(() => {
    setAuthSession({ access_token: 'token', token_type: 'Bearer', expires_in: 900 })
    vi.stubGlobal('scrollTo', vi.fn())
  })
  afterEach(() => { vi.unstubAllGlobals(); clearAuthToken() })

  it('月合計と明細合計を区別し、カテゴリ表と全明細を表示する', async () => {
    mockFetch({ '/api/monthly-summaries': summaries, '/api/months/2026-09/expenses': expenses })
    renderPage()
    expect(screen.getByRole('status', { name: '月別支出を読み込み中' })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { level: 1, name: '月別支出' })).toBeInTheDocument()
    expect(screen.getByText(/^[¥￥]12,000$/)).toBeInTheDocument()
    expect(screen.getByText('明細合計').parentElement).toHaveTextContent(/^[^¥￥]*[¥￥]13,500/)
    expect(screen.getByRole('rowheader', { name: /食費/ })).toBeInTheDocument()
    expect(screen.getByText('55.6%')).toBeInTheDocument()
    expect(screen.getByText('9/12 · 日用品ストア')).toBeInTheDocument()
    expect(screen.getByText('日用品 · 数量1 · 編集済み')).toBeInTheDocument()
    expect(screen.getAllByRole('listitem')).toHaveLength(3)
  })

  it('明細から支出詳細へ遷移し、戻り先の月と明細IDを渡す', async () => {
    mockFetch({ '/api/monthly-summaries': summaries, '/api/months/2026-09/expenses': expenses })
    const router = renderPage(); const user = userEvent.setup()
    await user.click(await screen.findByRole('link', { name: /米 5kg/ }))
    expect(screen.getByRole('heading', { name: '支出詳細' })).toBeInTheDocument()
    expect(router.state.location.pathname).toBe('/expenses/e1')
    expect(router.state.location.state).toEqual(expect.objectContaining({ from: '/months/2026-09', detailId: 'd1' }))
  })

  it('集計だけ失敗しても明細を残し、領域単位の再試行を表示する', async () => {
    mockFetch({ '/api/monthly-summaries': () => jsonResponse({ error: 'boom' }, 500), '/api/months/2026-09/expenses': expenses })
    renderPage()
    expect(await screen.findByRole('alert')).toHaveTextContent('月合計を取得できませんでした。')
    expect(screen.getByText('米 5kg')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '月合計を再読み込み' })).toBeEnabled()
  })

  it('集計も明細も0件なら取り込みへ進める空状態を表示する', async () => {
    mockFetch({ '/api/monthly-summaries': { monthly_summaries: [] }, '/api/months/2026-09/expenses': { year_month: '2026-09', items: [] } })
    renderPage()
    expect(await screen.findByRole('heading', { name: 'この月の支出はまだありません' })).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
    expect(screen.getByRole('link', { name: /レシートを取り込む/ })).toHaveAttribute('href', '/')
  })

  it('集計と明細の両方が失敗した場合は一つの回復操作を表示する', async () => {
    mockFetch({
      '/api/monthly-summaries': () => jsonResponse({ error: 'boom' }, 500),
      '/api/months/2026-09/expenses': () => jsonResponse({ error: 'boom' }, 500),
    })
    renderPage()
    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('月別支出を読み込めませんでした。')
    expect(screen.getAllByRole('alert')).toHaveLength(1)
    expect(within(alert).getByRole('button', { name: '再読み込み' })).toBeEnabled()
  })

  it('不正な年月ではAPI結果を表示せず今月への導線を示す', async () => {
    mockFetch({})
    renderPage('/months/2026-13')
    expect(screen.getByRole('heading', { name: '指定された月を表示できません' })).toBeInTheDocument()
    await waitFor(() => expect(screen.getByRole('link', { name: '今月を開く' })).toBeInTheDocument())
  })
})
