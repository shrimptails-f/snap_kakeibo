import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import type { Plugin } from 'vite'

const sampleReceipt = readFileSync(fileURLToPath(new URL('./sample-receipt.svg', import.meta.url)), 'utf8')

const categories = ['food', 'daily_goods', 'medical', 'transport', 'utilities', 'entertainment', 'social', 'clothing', 'education', 'other', 'unknown'] as const
const extraDetails = [
  [{ category: 'medical', name: 'サンプル医薬品' }, { category: 'transport', name: 'サンプル交通費' }],
  [{ category: 'utilities', name: 'サンプル光熱費' }, { category: 'entertainment', name: 'サンプル娯楽費' }],
  [{ category: 'social', name: 'サンプル交際費' }, { category: 'clothing', name: 'サンプル衣料品' }],
  [{ category: 'education', name: 'サンプル教材' }, { category: 'other', name: 'サンプル雑費' }],
  [{ category: 'unknown', name: 'サンプル未分類' }, { category: 'medical', name: 'サンプル医薬品' }],
  [{ category: 'transport', name: 'サンプル交通費' }, { category: 'utilities', name: 'サンプル光熱費' }],
  [{ category: 'entertainment', name: 'サンプル娯楽費' }, { category: 'social', name: 'サンプル交際費' }],
  [{ category: 'clothing', name: 'サンプル衣料品' }, { category: 'education', name: 'サンプル教材' }],
] as const

function monthAt(offset: number): string {
  const now = new Date()
  const date = new Date(now.getFullYear(), now.getMonth() - offset, 1)
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}`
}

function receiptForMonth(offset: number) {
  const yearMonth = monthAt(offset)
  const suffix = yearMonth.replace('-', '')
  const food = 800 + offset * 90
  const dailyGoods = 700 + offset * 60
  const firstExtraAmount = 350 + offset * 40
  const secondExtraAmount = 600 + offset * 55
  const recordedAmount = food + dailyGoods + firstExtraAmount + secondExtraAmount
  const updatedAt = new Date().toISOString()
  const expenseId = `sample-${suffix}`
  const analysisRequestId = `sample-request-${suffix}`
  return {
    expense: {
      expense_id: expenseId,
      analysis_request_id: analysisRequestId,
      store_name: 'Sample マーケット',
      purchase_date: `${yearMonth}-15`,
      year_month: yearMonth,
      read_amount: recordedAmount + 50,
      adjustment_amount: -50,
      recorded_amount: recordedAmount,
      source: 'AI',
      is_edited: false,
      updated_at: updatedAt,
    },
    details: [
      { detail_id: `sample-food-${suffix}`, name: 'サンプル食品', category: 'food', category_source: 'AI', amount: food, tax_included_amount: food, tax_mode: 'included', quantity: 2, source: 'AI', is_edited: false },
      { detail_id: `sample-daily-${suffix}`, name: 'サンプル日用品', category: 'daily_goods', category_source: 'AI', amount: dailyGoods, tax_included_amount: dailyGoods, tax_mode: 'included', quantity: 1, source: 'AI', is_edited: false },
      ...extraDetails[offset].map(({ category, name }, index) => ({ detail_id: `sample-extra-${index}-${suffix}`, name, category, category_source: 'AI', amount: index === 0 ? firstExtraAmount : secondExtraAmount, tax_included_amount: index === 0 ? firstExtraAmount : secondExtraAmount, tax_mode: 'included', quantity: 1, source: 'AI', is_edited: false })),
    ],
  }
}

function respond(res: import('node:http').ServerResponse, body: unknown, status = 200) {
  res.statusCode = status
  res.setHeader('Content-Type', 'application/json; charset=utf-8')
  res.end(JSON.stringify(body))
}

function receiptImage(receipt: ReturnType<typeof receiptForMonth>): string {
  const amount = (value: number) => `¥${value.toLocaleString('ja-JP')}`
  return receipt.details.reduce((svg, detail, index) => svg
    .replaceAll(`{{name${index}}}`, detail.name)
    .replaceAll(`{{amount${index}}}`, amount(detail.amount)), sampleReceipt)
    .replaceAll('{{subtotal}}', amount(receipt.expense.recorded_amount))
}

export function sampleApi(): Plugin {
  const receipts = Array.from({ length: 8 }, (_, index) => receiptForMonth(index))

  return {
    name: 'sample-api',
    apply: 'serve',
    configureServer(server) {
      server.middlewares.use((req, res, next) => {
        const url = new URL(req.url ?? '/', 'http://localhost')
        const path = url.pathname
        if (req.method === 'GET' && path === '/sample-receipt.svg') {
          const receipt = receipts.find(({ expense }) => expense.expense_id === url.searchParams.get('id'))
          if (!receipt) return respond(res, { error: 'not found' }, 404)
          res.setHeader('Content-Type', 'image/svg+xml; charset=utf-8')
          return res.end(receiptImage(receipt))
        }
        if (!path.startsWith('/api/')) return next()

        if (req.method === 'POST' && (path === '/api/auth/refresh' || path === '/api/auth/login')) {
          return respond(res, { access_token: 'sample-access-token', token_type: 'Bearer', expires_in: 3600, user: { user_id: 'sample-user', email: 'sample@example.com' } })
        }
        if (req.method === 'GET' && path === '/api/auth/check') {
          return respond(res, { user: { user_id: 'sample-user', email: 'sample@example.com' } })
        }
        if (req.method === 'POST' && path === '/api/auth/logout') {
          res.statusCode = 204
          return res.end()
        }

        if (req.method === 'GET' && path === '/api/monthly-summaries') {
          return respond(res, { monthly_summaries: receipts.map(({ expense, details }) => {
            const categoryTotals = Object.fromEntries(categories.map((category) => [category, details.filter((detail) => detail.category === category).reduce((sum, detail) => sum + detail.amount, 0)]))
            return { year_month: expense.year_month, total_recorded_amount: expense.recorded_amount, expense_count: 1, detail_count: details.length, confirmed_detail_count: details.length, category_totals: categoryTotals, updated_at: expense.updated_at }
          }) })
        }

        const expensesMonth = path.match(/^\/api\/months\/(\d{4}-\d{2})\/expenses$/)
        if (req.method === 'GET' && expensesMonth) {
          const items = receipts.filter(({ expense }) => expense.year_month === expensesMonth[1]).flatMap(({ expense, details }) => details.map((detail) => ({
            detail_id: detail.detail_id, expense_id: expense.expense_id, name: detail.name, category: detail.category,
            amount: detail.amount, tax_included_amount: detail.tax_included_amount, tax_mode: detail.tax_mode, quantity: detail.quantity, source: detail.source, is_edited: detail.is_edited,
            store_name: expense.store_name, purchase_date: expense.purchase_date,
          })))
          return respond(res, { year_month: expensesMonth[1], items })
        }

        const expenseId = path.match(/^\/api\/expenses\/([^/]+)$/)
        if (req.method === 'GET' && expenseId) {
          const receipt = receipts.find(({ expense }) => expense.expense_id === expenseId[1])
          if (!receipt) return respond(res, { error: 'not found' }, 404)
          const origin = `http://${req.headers.host ?? 'localhost:5173'}`
          return respond(res, { expense: { ...receipt.expense, image_url: `${origin}/sample-receipt.svg?id=${receipt.expense.expense_id}` }, details: receipt.details })
        }

        const analysisMonth = path.match(/^\/api\/months\/(\d{4}-\d{2})\/analysis-requests$/)
        if (req.method === 'GET' && analysisMonth) {
          const filter = url.searchParams.get('filter') ?? 'all'
          const items = receipts.filter(({ expense }) => expense.year_month === analysisMonth[1] && (filter === 'all' || filter === 'succeeded')).map(({ expense }) => ({
            analysis_request_id: expense.analysis_request_id, expense_id: expense.expense_id, status: 'SUCCEEDED', attempt: 1,
            file_name: 'sample-receipt.svg', year_month: expense.year_month, upload_expires_at: expense.updated_at,
            store_name: expense.store_name, recorded_amount: expense.recorded_amount, created_at: expense.updated_at, updated_at: expense.updated_at,
          }))
          return respond(res, { items })
        }

        return respond(res, { error: 'sample API: unsupported endpoint' }, 404)
      })
    },
  }
}
