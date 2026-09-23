import { Suspense } from 'react'
import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryRouter, RouterProvider } from 'react-router'
import { clearAuthToken, setAuthSession } from '@/shared/auth/token'
import { jsonResponse, mockFetch } from '@/test/mockFetch'
import { renderWithQuery } from '@/test/renderWithQuery'
import { UploadPage } from './UploadPage'

const month = new Date().toISOString().slice(0, 7)
const listPath = `/api/months/${month}/analysis-requests`

function renderPage() {
  const router = createMemoryRouter([
    { path: '/upload', element: <UploadPage /> },
    { path: '/analysis-requests', element: <p>解析履歴へ移動済み</p> },
  ], { initialEntries: ['/upload'] })
  return renderWithQuery(<Suspense fallback="loading"><RouterProvider router={router} /></Suspense>)
}

describe('UploadPage', () => {
  beforeEach(() => {
    setAuthSession({ access_token: 'test-token', token_type: 'Bearer', expires_in: 900 })
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: vi.fn((file: File) => `blob:${file.name}`) })
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: vi.fn() })
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
    clearAuthToken()
  })

  it('複数画像を送信前に確認し、画像単位で外せる', async () => {
    const { calls } = mockFetch({ [listPath]: { items: [] } })
    renderPage()
    await screen.findByRole('heading', { name: 'レシートを取り込む' })
    const user = userEvent.setup()
    const first = new File(['first'], 'receipt-1.jpg', { type: 'image/jpeg' })
    const second = new File(['second'], 'receipt-2.png', { type: 'image/png' })

    await user.upload(screen.getByLabelText('画像を選ぶ（複数可）'), [first, second])

    expect(screen.getByRole('heading', { name: /選択した画像 2枚/ })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '2枚をアップロード' })).toBeEnabled()
    expect(calls.filter((call) => call.url === '/api/uploads')).toHaveLength(0)
    await user.click(screen.getAllByRole('button', { name: '外す' })[0])
    expect(screen.queryByText('receipt-1.jpg')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '1枚をアップロード' })).toBeEnabled()
  })

  it('一部の画像送信が失敗しても他を継続し、完了した画像から支出詳細へ進める', async () => {
    let listCount = 0
    let createCount = 0
    const succeeded = {
      analysis_request_id: 'req1', expense_id: 'expense1', status: 'SUCCEEDED', attempt: 1,
      file_name: 'receipt-1.jpg', year_month: month, upload_expires_at: '2099-09-22T12:15:00Z',
      created_at: '2026-09-22T12:00:00Z', updated_at: '2026-09-22T12:01:00Z',
    }
    const uploadPending = { ...succeeded, analysis_request_id: 'req2', expense_id: undefined, status: 'UPLOADING', file_name: 'receipt-2.png' }
    mockFetch({
      [listPath]: () => jsonResponse({ items: ++listCount === 1 ? [] : [succeeded, uploadPending] }),
      '/api/uploads': () => {
        createCount += 1
        return jsonResponse({ analysis_request_id: `req${createCount}`, post_url: `https://s3.example.com/${createCount}`, post_fields: { key: `receipts/req${createCount}/original.jpg`, policy: 'signed' }, expires_at: '2099-09-22T12:15:00Z' })
      },
      'https://s3.example.com/1': () => new Response(null, { status: 200 }),
      'https://s3.example.com/2': () => new Response(null, { status: 500 }),
    })
    renderPage()
    await screen.findByRole('heading', { name: 'レシートを取り込む' })
    const user = userEvent.setup()
    await user.upload(screen.getByLabelText('画像を選ぶ（複数可）'), [
      new File(['first'], 'receipt-1.jpg', { type: 'image/jpeg' }),
      new File(['second'], 'receipt-2.png', { type: 'image/png' }),
    ])
    await user.click(screen.getByRole('button', { name: '2枚をアップロード' }))

    expect(await screen.findByRole('link', { name: /支出詳細を見る/ })).toHaveAttribute('href', '/expenses/expense1?from=upload&request=req1')
    expect(screen.getByText('画像を送信できませんでした。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '状態を確認' })).toBeInTheDocument()
    await waitFor(() => expect(screen.getByText('登録完了1 / 要対応1')).toBeInTheDocument())
    expect(screen.queryByText(/画像の送信が終わりました/)).not.toBeInTheDocument()
  })

  it('画像以外は理由を示して送信対象から外す', async () => {
    mockFetch({ [listPath]: { items: [] } })
    renderPage()
    await screen.findByRole('heading', { name: 'レシートを取り込む' })
    const user = userEvent.setup({ applyAccept: false })
    await user.upload(screen.getByLabelText('画像を選ぶ（複数可）'), new File(['pdf'], 'receipt.pdf', { type: 'application/pdf' }))
    expect(screen.getByText('JPEG または PNG の画像を選んでください。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '0枚をアップロード' })).toBeDisabled()
  })

  it('29 MiB を超える画像は送信前に拒否する', async () => {
    const { calls } = mockFetch({ [listPath]: { items: [] } })
    renderPage()
    await screen.findByRole('heading', { name: 'レシートを取り込む' })
    const file = new File([new Uint8Array((29 << 20) + 1)], 'large.jpg', { type: 'image/jpeg' })
    await userEvent.setup().upload(screen.getByLabelText('画像を選ぶ（複数可）'), file)
    expect(screen.getByText('画像は 29 MiB 以下にしてください。')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '0枚をアップロード' })).toBeDisabled()
    expect(calls.filter((call) => call.url === '/api/uploads')).toHaveLength(0)
  })

  it('未送信の画像があるときはアプリ内の移動前に確認する', async () => {
    mockFetch({ [listPath]: { items: [] } })
    renderPage()
    await screen.findByRole('heading', { name: 'レシートを取り込む' })
    const user = userEvent.setup()
    await user.upload(screen.getByLabelText('画像を選ぶ（複数可）'), new File(['receipt'], 'receipt.jpg', { type: 'image/jpeg' }))
    await user.click(screen.getByRole('link', { name: /解析履歴を見る/ }))
    expect(screen.getByRole('alertdialog')).toHaveTextContent('未送信の画像があります。')
    await user.click(screen.getByRole('button', { name: '移動する' }))
    expect(await screen.findByText('解析履歴へ移動済み')).toBeInTheDocument()
  })
})
