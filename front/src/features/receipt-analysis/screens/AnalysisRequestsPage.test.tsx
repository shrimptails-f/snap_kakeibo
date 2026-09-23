import { Suspense } from 'react'
import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryRouter, MemoryRouter, RouterProvider } from 'react-router'
import { clearAuthToken, setAuthSession } from '@/shared/auth/token'
import { jsonResponse, mockFetch } from '@/test/mockFetch'
import { renderWithQuery } from '@/test/renderWithQuery'
import { AnalysisRequestsPage } from './AnalysisRequestsPage'

const month = new Date().toISOString().slice(0, 7)
const listPath = `/api/months/${month}/analysis-requests`

describe('AnalysisRequestsPage', () => {
  beforeEach(() => setAuthSession({ access_token: 'test-token', token_type: 'Bearer', expires_in: 900 }))
  afterEach(() => { vi.unstubAllGlobals(); clearAuthToken() })

  it('停滞した解析を確認後に再解析し、受付結果を案内する', async () => {
    const stalled = {
      analysis_request_id: 'req1', status: 'ANALYZING', attempt: 1, file_name: 'receipt.jpg', year_month: month,
      upload_expires_at: '2020-01-01T00:15:00Z', created_at: '2020-01-01T00:00:00Z', updated_at: '2020-01-01T00:01:00Z',
    }
    mockFetch({
      [listPath]: { items: [stalled] },
      '/api/analysis-requests/req1/retry': () => jsonResponse({ analysis_request_id: 'req1', status: 'ANALYZING', attempt: 2 }),
    })
    renderWithQuery(<MemoryRouter><Suspense fallback="loading"><AnalysisRequestsPage /></Suspense></MemoryRouter>)
    const user = userEvent.setup()
    expect(await screen.findByText('停滞')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'receipt.jpgを再解析する' }))
    const dialog = screen.getByRole('alertdialog')
    expect(dialog).toHaveTextContent('前の解析が進行中の可能性があります。')
    await user.click(within(dialog).getByRole('button', { name: '再解析する' }))
    expect(await screen.findByRole('status')).toHaveTextContent('再解析を開始しました。')
  })

  it('再読み込みに失敗しても直前の一覧を残して案内する', async () => {
    let count = 0
    const completed = {
      analysis_request_id: 'req2', expense_id: 'expense2', status: 'SUCCEEDED', attempt: 1, file_name: 'done.jpg', year_month: month,
      upload_expires_at: '2099-01-01T00:15:00Z', created_at: '2026-09-22T00:00:00Z', updated_at: '2026-09-22T00:01:00Z',
    }
    mockFetch({ [listPath]: () => ++count === 1 ? jsonResponse({ items: [completed] }) : jsonResponse({ error: 'internal' }, 500) })
    renderWithQuery(<MemoryRouter><Suspense fallback="loading"><AnalysisRequestsPage /></Suspense></MemoryRouter>)
    expect(await screen.findByText('done.jpg')).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '再読み込み' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('最新の状態を取得できませんでした。')
    expect(screen.getByText('done.jpg')).toBeInTheDocument()
  })

  it('店舗名と計上額を表示し、カーソルで次ページと前ページへ移動する', async () => {
    const first = {
      analysis_request_id: 'req1', expense_id: 'expense1', status: 'SUCCEEDED', attempt: 1, file_name: 'first.jpg', year_month: month,
      store_name: 'テストスーパー', recorded_amount: 2780,
      upload_expires_at: '2099-01-01T00:15:00Z', created_at: '2026-09-22T00:00:00Z', updated_at: '2026-09-22T00:01:00Z',
    }
    const second = { ...first, analysis_request_id: 'req2', expense_id: undefined, status: 'FAILED', error_code: 'INTERNAL', store_name: undefined, recorded_amount: undefined, file_name: 'second.jpg' }
    const { calls } = mockFetch({
      [listPath]: ({ url }) => new URL(url, 'http://test').searchParams.get('cursor') === 'next-1'
        ? jsonResponse({ items: [second] })
        : jsonResponse({ items: [first], next_cursor: 'next-1' }),
    })
    const router = createMemoryRouter([{ path: '/analysis-requests', element: <AnalysisRequestsPage /> }], { initialEntries: ['/analysis-requests'] })
    renderWithQuery(<Suspense fallback="loading"><RouterProvider router={router} /></Suspense>)
    const user = userEvent.setup()
    expect(await screen.findByText('テストスーパー')).toBeInTheDocument()
    expect(screen.getByText(/2,780/)).toBeInTheDocument()
    await user.click(screen.getAllByRole('button', { name: '店舗・計上額' })[0])
    expect(screen.getByRole('tooltip')).toHaveTextContent('最終支払合計に利用者調整額を反映した金額')
    await user.keyboard('{Escape}')
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '次へ' }))
    expect(await screen.findByText('second.jpg')).toBeInTheDocument()
    expect(router.state.location.search).toContain('cursor=next-1')
    expect(router.state.location.search).toContain('page=2')
    await user.click(screen.getByRole('button', { name: '前へ' }))
    expect(await screen.findByText('first.jpg')).toBeInTheDocument()
    expect(router.state.location.search).not.toContain('cursor=')
    expect(calls.some(({ url }) => url.includes('filter=all') && url.includes('cursor=next-1'))).toBe(true)
  })

  it('受付月と状態をURLへ保持して月全体の絞り込みをAPIへ渡す', async () => {
    const { calls } = mockFetch({ [listPath]: { items: [] } })
    const router = createMemoryRouter([{ path: '/analysis-requests', element: <AnalysisRequestsPage /> }], { initialEntries: [`/analysis-requests?month=${month}&filter=all`] })
    renderWithQuery(<Suspense fallback="loading"><RouterProvider router={router} /></Suspense>)
    const user = userEvent.setup()
    await screen.findByText('この受付月の取り込みはありません。')
    await user.selectOptions(screen.getByRole('combobox', { name: '表示' }), 'attention')
    await screen.findByText('要対応の画像はありません。')
    expect(router.state.location.search).toContain('filter=attention')
    expect(calls.some(({ url }) => url.includes('filter=attention'))).toBe(true)
  })

  it('次ページの取得失敗時は元のページとURLを維持する', async () => {
    const item = {
      analysis_request_id: 'req1', status: 'UPLOADING', attempt: 1, file_name: 'kept.jpg', year_month: month,
      upload_expires_at: '2099-01-01T00:15:00Z', created_at: '2026-09-22T00:00:00Z', updated_at: '2026-09-22T00:01:00Z',
    }
    mockFetch({
      [listPath]: ({ url }) => new URL(url, 'http://test').searchParams.has('cursor')
        ? jsonResponse({ error: 'internal' }, 500)
        : jsonResponse({ items: [item], next_cursor: 'broken-next' }),
    })
    const router = createMemoryRouter([{ path: '/analysis-requests', element: <AnalysisRequestsPage /> }], { initialEntries: ['/analysis-requests'] })
    renderWithQuery(<Suspense fallback="loading"><RouterProvider router={router} /></Suspense>)
    await screen.findByText('kept.jpg')
    await userEvent.click(screen.getByRole('button', { name: '次へ' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('元のページを表示しています')
    expect(screen.getByText('kept.jpg')).toBeInTheDocument()
    expect(router.state.location.search).not.toContain('cursor=')
  })

  it('初回取得失敗時も画面見出しと取り込み導線を残して再試行できる', async () => {
    let count = 0
    mockFetch({ [listPath]: () => ++count === 1 ? jsonResponse({ error: 'internal' }, 500) : jsonResponse({ items: [] }) })
    renderWithQuery(<MemoryRouter><AnalysisRequestsPage /></MemoryRouter>)
    expect(await screen.findByRole('heading', { name: '解析履歴を読み込めませんでした' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: '解析履歴', level: 1 })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'レシートを取り込む' })).toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: '再読み込み' }))
    expect(await screen.findByText('この受付月の取り込みはありません。')).toBeInTheDocument()
  })
})
