import { act, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clearAuthToken, setAuthSession } from '@/shared/auth/token'
import { formatYen } from '@/shared/lib/formatYen'
import { jsonResponse, mockFetch } from '@/test/mockFetch'
import { renderWithQuery } from '@/test/renderWithQuery'
import { ReceiptIntakePage } from './ReceiptIntakePage'

const month = new Date().toISOString().slice(0, 7)
const listPath = `/api/months/${month}/analysis-requests`

const succeededItem = {
  analysis_request_id: 'req1',
  expense_id: 'e1',
  status: 'SUCCEEDED',
  attempt: 1,
  file_name: 'receipt-1.jpg',
  year_month: month,
  upload_expires_at: '2026-09-18T00:15:00Z',
  created_at: '2026-09-18T00:00:00Z',
  updated_at: '2026-09-18T00:00:00Z',
}

const expenseResponse = {
  expense: {
    expense_id: 'e1',
    analysis_request_id: 'req1',
    store_name: '居酒屋',
    purchase_date: '2026-09-18',
    year_month: '2026-09',
    read_amount: 5000,
    adjustment_amount: -2500,
    recorded_amount: 2500,
    source: 'AI',
    is_edited: true,
    updated_at: '2026-09-18T01:00:00Z',
  },
  details: [{ detail_id: 'd1', name: '飲み会', category: 'social', category_source: 'AI', amount: 5000, quantity: 1, source: 'AI', is_edited: true }],
}

describe('ReceiptIntakePage', () => {
  beforeEach(() => {
    // 再読み込みのクールタイム(5 秒)を進めるために fake timers を使う
    vi.useFakeTimers({ shouldAdvanceTime: true })
    setAuthSession({ access_token: 'test-token', token_type: 'Bearer', expires_in: 900 })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    clearAuthToken()
  })

  it('初回取得が終わるまでスピナーを出し、空なら案内を表示する', async () => {
    const { fetchMock } = mockFetch({ [listPath]: { items: [] } })

    renderWithQuery(<ReceiptIntakePage />)

    expect(screen.getByRole('status', { name: '画面を読み込んでいます' })).toBeInTheDocument()
    expect(screen.queryByText('まだ解析依頼がありません。')).not.toBeInTheDocument()
    expect(await screen.findByText('まだ解析依頼がありません。')).toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledWith(
      listPath,
      expect.objectContaining({ headers: { Authorization: 'Bearer test-token' } }),
    )
  })

  it('解析依頼を一覧に表示し、登録完了していない行は選択できない', async () => {
    mockFetch({
      [listPath]: {
        items: [
          succeededItem,
          { ...succeededItem, analysis_request_id: 'req2', expense_id: undefined, status: 'ANALYZING', attempt: 2, file_name: 'receipt-2.jpg' },
          {
            ...succeededItem,
            analysis_request_id: 'req3',
            expense_id: undefined,
            status: 'UPLOADING',
            file_name: 'receipt-3.jpg',
            upload_expires_at: '2020-01-01T00:00:00Z',
          },
        ],
      },
    })

    renderWithQuery(<ReceiptIntakePage />)

    const done = await screen.findByRole('button', { name: /receipt-1\.jpg/ })
    const analyzing = screen.getByRole('button', { name: /receipt-2\.jpg/ })
    expect(done).toBeEnabled()
    expect(analyzing).toBeDisabled()
    expect(screen.getByText('期限切れ')).toBeInTheDocument()
    expect(screen.getByText('試行 2 回目')).toBeInTheDocument()
  })

  it('再読み込みで一覧を再取得し、そのあと 5 秒間は再度押せない', async () => {
    let listCount = 0
    mockFetch({
      [listPath]: () => {
        listCount += 1
        return jsonResponse({ items: listCount === 1 ? [succeededItem] : [succeededItem, { ...succeededItem, analysis_request_id: 'req2', file_name: 'receipt-2.jpg' }] })
      },
    })

    renderWithQuery(<ReceiptIntakePage />)
    await screen.findByRole('button', { name: /receipt-1\.jpg/ })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })

    await user.click(screen.getByRole('button', { name: '再読み込み' }))

    // 再取得中も前回の一覧は表示したまま
    expect(screen.getByRole('button', { name: /receipt-1\.jpg/ })).toBeInTheDocument()
    expect(await screen.findByRole('button', { name: /receipt-2\.jpg/ })).toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    expect(listCount).toBe(2)

    // クールタイム中は押しても API を呼ばない
    const reload = screen.getByRole('button', { name: '再読み込み' })
    expect(reload).toBeDisabled()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(4000)
    })
    expect(reload).toBeDisabled()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000)
    })
    expect(reload).toBeEnabled()
    expect(listCount).toBe(2)
  })

  it('登録完了した解析依頼を選ぶと、支出パネルだけスピナーを出してから支出と明細を表示する', async () => {
    mockFetch({ [listPath]: { items: [succeededItem] }, '/api/expenses/e1': expenseResponse })

    renderWithQuery(<ReceiptIntakePage />)

    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    await user.click(await screen.findByRole('button', { name: /receipt-1\.jpg/ }))

    expect(screen.getByRole('status', { name: '支出を読み込んでいます' })).toBeInTheDocument()
    // 一覧は表示したまま
    expect(screen.getByRole('button', { name: /receipt-1\.jpg/ })).toBeInTheDocument()

    expect(await screen.findByText('居酒屋')).toBeInTheDocument()
    expect(screen.getByText(formatYen(2500))).toBeInTheDocument()
    expect(screen.getByText(`読取金額 ${formatYen(5000)} / 調整額 ${formatYen(-2500)}`)).toBeInTheDocument()
    expect(screen.getByText('交際・会食')).toBeInTheDocument()
    expect(screen.getByText(/AI由来・手動編集済み/)).toBeInTheDocument()
    expect(screen.getByText('（手動編集済み）')).toBeInTheDocument()
  })

  it('支出の取得に失敗したらパネルの中で案内し、再試行できる', async () => {
    let expenseCount = 0
    mockFetch({
      [listPath]: { items: [succeededItem] },
      '/api/expenses/e1': () => {
        expenseCount += 1
        return expenseCount === 1 ? jsonResponse({ error: 'internal' }, 500) : jsonResponse(expenseResponse)
      },
    })

    renderWithQuery(<ReceiptIntakePage />)
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    await user.click(await screen.findByRole('button', { name: /receipt-1\.jpg/ }))

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('サーバーでエラーが発生しました。時間をおいて再度お試しください。')
    expect(alert).not.toHaveTextContent('500')
    expect(screen.getByRole('button', { name: /receipt-1\.jpg/ })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '再試行' }))

    expect(await screen.findByText('居酒屋')).toBeInTheDocument()
  })

  it('ファイルを選ぶと Presigned URL へ PUT し、完了を通知して一覧を再取得する', async () => {
    let listCount = 0
    const { calls } = mockFetch({
      [listPath]: () => {
        listCount += 1
        return jsonResponse({ items: listCount === 1 ? [] : [succeededItem] })
      },
      '/api/uploads': { put_url: 'https://s3.example.com/put', analysis_request_id: 'req1' },
      'https://s3.example.com/put': () => new Response(null, { status: 200 }),
    })

    renderWithQuery(<ReceiptIntakePage />)
    await screen.findByText('まだ解析依頼がありません。')

    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    const file = new File(['receipt'], 'receipt-1.jpg', { type: 'image/jpeg' })
    await user.upload(screen.getByLabelText('ファイルを選択'), file)

    expect(await screen.findByRole('status')).toHaveTextContent('アップロードしました。')
    expect(await screen.findByRole('button', { name: /receipt-1\.jpg/ })).toBeInTheDocument()
    const putCall = calls.find((call) => call.url === 'https://s3.example.com/put')
    expect(putCall?.init.method).toBe('PUT')
    expect(putCall?.init.body).toBe(file)
  })

  it('アップロードに失敗したら HTTP ステータスを出さずに利用者向けの文言を表示する', async () => {
    mockFetch({
      [listPath]: { items: [] },
      '/api/uploads': () => jsonResponse({ error: 'internal' }, 500),
    })

    renderWithQuery(<ReceiptIntakePage />)
    await screen.findByText('まだ解析依頼がありません。')

    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    await user.upload(screen.getByLabelText('ファイルを選択'), new File(['receipt'], 'receipt-1.jpg', { type: 'image/jpeg' }))

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('サーバーでエラーが発生しました。')
    expect(alert).not.toHaveTextContent('500')
    await waitFor(() => expect(screen.getByLabelText('ファイルを選択')).toBeEnabled())
  })
})
