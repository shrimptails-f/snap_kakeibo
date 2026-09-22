import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createQueryClient } from '@/app/providers/queryClient'
import { clearAuthToken, setAuthSession } from '@/shared/auth/token'
import { jsonResponse, mockFetch } from '@/test/mockFetch'
import { ExpenseDetailPage } from './ExpenseDetailPage'

const original = {
  expense: { expense_id: 'e1', analysis_request_id: 'r1', store_name: 'スーパー', purchase_date: '2026-09-15', year_month: '2026-09', read_amount: 3280, adjustment_amount: -500, recorded_amount: 2780, source: 'AI', is_edited: false, updated_at: '2026-09-15T12:10:00Z' },
  details: [
    { detail_id: 'd1', name: '牛乳', category: 'food', category_source: 'AI', amount: 281, quantity: 1, source: 'AI', is_edited: false },
    { detail_id: 'd2', name: 'お米', category: 'food', category_source: 'AI', amount: 2999, quantity: 1, source: 'AI', is_edited: false },
  ],
}
const updated = { ...original, expense: { ...original.expense, adjustment_amount: -1000, recorded_amount: 2280, source: 'USER', is_edited: true, updated_at: '2026-09-22T01:00:00Z' } }

function renderPage() {
  const router = createMemoryRouter([{ path: '/expenses/:expenseId', element: <ExpenseDetailPage /> }, { path: '/months/:month', element: <h1>月別支出</h1> }], { initialEntries: ['/expenses/e1'] })
  render(<QueryClientProvider client={createQueryClient()}><RouterProvider router={router} /></QueryClientProvider>)
  return router
}

describe('ExpenseDetailPage', () => {
  beforeEach(() => setAuthSession({ access_token: 'token', token_type: 'Bearer', expires_in: 900 }))
  afterEach(() => { vi.unstubAllGlobals(); clearAuthToken() })

  it('支出の金額・由来・明細と画像未対応の案内を表示する', async () => {
    mockFetch({ '/api/expenses/e1': original })
    renderPage()
    const user = userEvent.setup()
    expect(screen.getByRole('status', { name: '支出を読み込み中' })).toBeInTheDocument()
    expect(await screen.findByRole('heading', { level: 1, name: '支出詳細' })).toBeInTheDocument()
    expect(screen.getByText(/^[¥￥]2,780$/)).toBeInTheDocument()
    expect(screen.getByText('レシート画像は現在表示できません。')).toBeInTheDocument()
    expect(screen.getAllByText('AI解析')).toHaveLength(3)
    await user.click(screen.getAllByRole('button', { name: 'AI解析について' })[0])
    expect(screen.getByRole('tooltip')).toHaveTextContent('AIがレシートを解析した結果です。')
    expect(screen.getByRole('button', { name: '編集する' })).toBeEnabled()
  })

  it('入力中に計上額を再計算し、保存後のGETで閲覧表示を確定する', async () => {
    let getCount = 0
    const { calls } = mockFetch({ '/api/expenses/e1': ({ init }) => { if (init.method === 'PATCH') return jsonResponse({ expense: { expense_id: 'e1', read_amount: 3280, adjustment_amount: -1000, recorded_amount: 2280, updated_at: '2026-09-22T01:00:00Z' } }); getCount += 1; return jsonResponse(getCount === 1 ? original : updated) } })
    renderPage(); const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: '編集する' }))
    const adjustment = screen.getByLabelText(/調整額（必須）/)
    await user.clear(adjustment); await user.type(adjustment, '-1000')
    expect(screen.getAllByText(/^[¥￥]2,280$/).length).toBeGreaterThan(0)
    await user.click(screen.getByRole('button', { name: '変更を保存' }))
    expect(await screen.findByRole('status')).toHaveTextContent('変更を保存しました。')
    expect(screen.getByRole('heading', { level: 1, name: '支出詳細' })).toBeInTheDocument()
    const patchCall = calls.find((call) => call.init.method === 'PATCH')
    expect(JSON.parse(String(patchCall?.init.body))).toEqual(expect.objectContaining({ adjustment_amount: -1000, details: expect.arrayContaining([expect.objectContaining({ detail_id: 'd1' })]) }))
    expect(getCount).toBe(2)
  })

  it('保存失敗時は入力を残して手動で再試行できる', async () => {
    mockFetch({ '/api/expenses/e1': ({ init }) => init.method === 'PATCH' ? jsonResponse({ error: 'internal' }, 500) : jsonResponse(original) })
    renderPage(); const user = userEvent.setup(); await user.click(await screen.findByRole('button', { name: '編集する' }))
    const store = screen.getByLabelText('店舗名（必須）'); await user.clear(store); await user.type(store, '別の店')
    await user.click(screen.getByRole('button', { name: '変更を保存' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('変更を保存できませんでした。入力内容は残っています。')
    expect(store).toHaveValue('別の店')
    expect(screen.getByRole('button', { name: '変更を保存' })).toBeEnabled()
  })

  it('未保存のキャンセルでは破棄確認を表示する', async () => {
    mockFetch({ '/api/expenses/e1': original }); renderPage(); const user = userEvent.setup(); await user.click(await screen.findByRole('button', { name: '編集する' }))
    await user.type(screen.getByLabelText('店舗名（必須）'), '追記')
    await user.click(screen.getByRole('button', { name: 'キャンセル' }))
    expect(screen.getByRole('dialog', { name: '変更を破棄しますか？' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '変更を破棄する' }))
    await waitFor(() => expect(screen.getByRole('heading', { level: 1, name: '支出詳細' })).toBeInTheDocument())
  })
})
