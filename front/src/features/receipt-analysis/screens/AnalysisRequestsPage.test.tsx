import { Suspense } from 'react'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MemoryRouter } from 'react-router'
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
    await user.click(screen.getByRole('button', { name: '再解析する' }))
    expect(screen.getByRole('alertdialog')).toHaveTextContent('前の解析が進行中の可能性があります。')
    await user.click(screen.getAllByRole('button', { name: '再解析する' })[1])
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
})
