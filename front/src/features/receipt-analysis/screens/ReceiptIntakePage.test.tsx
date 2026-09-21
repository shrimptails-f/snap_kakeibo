import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { clearAuthToken, setAuthSession } from '@/shared/auth/token'
import { formatYen } from '@/shared/lib/formatYen'
import { jsonResponse, mockFetch } from '@/test/mockFetch'
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

describe('ReceiptIntakePage', () => {
  beforeEach(() => {
    // 5 秒ごとの再取得タイマーがテスト終了後に走らないようにする
    vi.useFakeTimers({ shouldAdvanceTime: true })
    setAuthSession({ access_token: 'test-token', token_type: 'Bearer', expires_in: 900 })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
    clearAuthToken()
  })

  it('解析依頼が空のときは案内を表示する', async () => {
    const { fetchMock } = mockFetch({ [listPath]: { items: [] } })

    render(<ReceiptIntakePage />)

    expect(await screen.findByText('まだ解析依頼がありません。')).toBeInTheDocument()
    await waitFor(() =>
      expect(fetchMock).toHaveBeenCalledWith(
        listPath,
        expect.objectContaining({ headers: { Authorization: 'Bearer test-token' } }),
      ),
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

    render(<ReceiptIntakePage />)

    const done = await screen.findByRole('button', { name: /receipt-1\.jpg/ })
    const analyzing = screen.getByRole('button', { name: /receipt-2\.jpg/ })
    expect(done).toBeEnabled()
    expect(analyzing).toBeDisabled()
    expect(screen.getByText('期限切れ')).toBeInTheDocument()
    expect(screen.getByText('試行 2 回目')).toBeInTheDocument()
  })

  it('登録完了した解析依頼を選ぶと支出と明細をカテゴリの表示名付きで表示する', async () => {
    mockFetch({
      [listPath]: { items: [succeededItem] },
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

    render(<ReceiptIntakePage />)

    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    await user.click(await screen.findByRole('button', { name: /receipt-1\.jpg/ }))

    expect(await screen.findByText('居酒屋')).toBeInTheDocument()
    expect(screen.getByText(formatYen(2500))).toBeInTheDocument()
    expect(screen.getByText(`読取金額 ${formatYen(5000)} / 調整額 ${formatYen(-2500)}`)).toBeInTheDocument()
    expect(screen.getByText('交際・会食')).toBeInTheDocument()
    expect(screen.getByText(/AI由来・手動編集済み/)).toBeInTheDocument()
    expect(screen.getByText('（手動編集済み）')).toBeInTheDocument()
  })

  it('取得に失敗したら HTTP ステータスを出さずに利用者向けの文言を表示する', async () => {
    mockFetch({ [listPath]: () => jsonResponse({ error: 'internal' }, 500) })

    render(<ReceiptIntakePage />)

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('サーバーでエラーが発生しました。時間をおいて再度お試しください。')
    expect(alert).not.toHaveTextContent('500')
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

    render(<ReceiptIntakePage />)
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
})
