import { QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
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
const withImage = { ...original, expense: { ...original.expense, image_url: 'https://example.com/receipt.jpg' } }

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
    expect(screen.getAllByText('AI解析')).toHaveLength(2)
    await user.click(screen.getAllByRole('button', { name: 'AI解析' })[0])
    expect(screen.getByRole('tooltip')).toHaveTextContent('AIがレシートを解析した結果です。')
    expect(screen.getByRole('button', { name: '編集する' })).toBeEnabled()
  })

  it('明細の税込み額と印字額を区別し、未確定を示す', async () => {
    const withTax = { ...original, details: [{ ...original.details[0], amount: 281, tax_included_amount: 303, tax_rate: 8, tax_mode: 'external' }, original.details[1]] }
    mockFetch({ '/api/expenses/e1': withTax })
    renderPage()
    expect(await screen.findByText(/税込み明細額（印字額 [¥￥]281） \/ 税率 8% \/ 配分税額 [¥￥]22/)).toBeInTheDocument()
    expect(screen.getByText('印字額・税込み未確定')).toBeInTheDocument()
    expect(screen.getByText(/税込み額未確定の明細 1件は印字額で含めています/)).toBeInTheDocument()
  })

  it('解析履歴の検索条件と復元情報を保って戻る', async () => {
    mockFetch({ '/api/expenses/e1': original })
    const router = createMemoryRouter([
      { path: '/expenses/:expenseId', element: <ExpenseDetailPage /> },
      { path: '/analysis-requests', element: <h1>解析履歴</h1> },
    ], { initialEntries: [{ pathname: '/expenses/e1', state: { from: '/analysis-requests?month=2026-09&filter=attention&cursor=next&page=2', backLabel: '解析履歴へ', requestId: 'req1', scrollY: 320, analysisCursorHistory: [''] } }] })
    render(<QueryClientProvider client={createQueryClient()}><RouterProvider router={router} /></QueryClientProvider>)
    await userEvent.click(await screen.findByRole('link', { name: /解析履歴へ/ }))
    expect(router.state.location.pathname).toBe('/analysis-requests')
    expect(router.state.location.search).toContain('filter=attention')
    expect(router.state.location.search).toContain('page=2')
    expect(router.state.location.state).toEqual(expect.objectContaining({ restoreRequestId: 'req1', scrollY: 320, analysisCursorHistory: [''] }))
  })

  it('アップロードから開いた支出は依頼IDを保って取り込みへ戻る', async () => {
    mockFetch({ '/api/expenses/e1': original })
    const router = createMemoryRouter([
      { path: '/expenses/:expenseId', element: <ExpenseDetailPage /> },
      { path: '/upload', element: <h1>レシートを取り込む</h1> },
    ], { initialEntries: ['/expenses/e1?from=upload&request=r1'] })
    render(<QueryClientProvider client={createQueryClient()}><RouterProvider router={router} /></QueryClientProvider>)
    await userEvent.click(await screen.findByRole('link', { name: /アップロードへ/ }))
    expect(router.state.location.pathname).toBe('/upload')
    expect(router.state.location.search).toBe('?request=r1')
  })

  it('レシート画像を表示し、拡大ダイアログを操作できる', async () => {
    mockFetch({ '/api/expenses/e1': withImage })
    renderPage(); const user = userEvent.setup()
    await screen.findByRole('heading', { level: 1, name: '支出詳細' })
    await user.click(screen.getByRole('button', { name: 'レシート画像を確認' }))
    const image = screen.getByRole('img', { name: 'この支出のレシート画像' })
    expect(image).toHaveAttribute('src', 'https://example.com/receipt.jpg')
    fireEvent.load(image)
    const expand = screen.getByRole('button', { name: '拡大して見る' })
    await user.click(expand)
    expect(screen.getByRole('dialog', { name: 'レシート画像' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '拡大' }))
    await user.click(screen.getByRole('button', { name: '全体を表示' }))
    await user.click(screen.getByRole('button', { name: '閉じる' }))
    expect(expand).toHaveFocus()
  })

  it('画像読込失敗時はGETだけを再試行して新しい署名URLを使う', async () => {
    let getCount = 0
    mockFetch({ '/api/expenses/e1': () => { getCount += 1; return jsonResponse({ ...withImage, expense: { ...withImage.expense, image_url: `https://example.com/receipt-${getCount}.jpg` } }) } })
    renderPage(); const user = userEvent.setup()
    await screen.findByRole('heading', { level: 1, name: '支出詳細' })
    await user.click(screen.getByRole('button', { name: 'レシート画像を確認' }))
    fireEvent.error(screen.getByRole('img', { name: 'この支出のレシート画像' }))
    expect(screen.getByRole('alert')).toHaveTextContent('画像を表示できません。')
    await user.click(screen.getByRole('button', { name: '画像を再読み込み' }))
    await waitFor(() => expect(screen.getByRole('img', { name: 'この支出のレシート画像' })).toHaveAttribute('src', 'https://example.com/receipt-2.jpg'))
    expect(getCount).toBe(2)
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

  it('明細を追加・削除し、保存後の明細に反映する', async () => {
    const replaced = { ...original, details: [original.details[1], { ...original.details[1], detail_id: 'd3', name: '新しい品', source: 'USER', category_source: 'USER', is_edited: true }] }
    let getCount = 0
    const { calls } = mockFetch({ '/api/expenses/e1': ({ init }) => init.method === 'PATCH'
      ? jsonResponse({ expense: { expense_id: 'e1', read_amount: 3280, adjustment_amount: -500, recorded_amount: 2780, updated_at: '2026-09-22T01:00:00Z' } })
      : jsonResponse(++getCount === 1 ? original : replaced) })
    renderPage(); const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: '編集する' }))
    await user.click(screen.getByRole('button', { name: '1件目の明細を削除' }))
    await user.click(screen.getByRole('button', { name: '明細を追加' }))
    const names = screen.getAllByLabelText('商品名（必須）')
    await user.type(names[1], '新しい品')
    await user.click(screen.getByRole('button', { name: '変更を保存' }))
    expect(await screen.findByRole('status')).toHaveTextContent('変更を保存しました。')
    const body = JSON.parse(String(calls.find((call) => call.init.method === 'PATCH')?.init.body))
    expect(body.details).toHaveLength(2)
    expect(body.details[0].detail_id).toBe('d2')
    expect(body.details[1]).toMatchObject({ name: '新しい品', amount: 0, quantity: 1, category: 'unknown' })
    expect(body.details[1].detail_id).toBeUndefined()
    expect(screen.getByText('新しい品')).toBeInTheDocument()
  })

  it('最後の明細を削除できない', async () => {
    mockFetch({ '/api/expenses/e1': { ...original, details: [original.details[0]] } })
    renderPage(); await userEvent.click(await screen.findByRole('button', { name: '編集する' }))
    expect(screen.getByRole('button', { name: '1件目の明細を削除' })).toBeDisabled()
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
