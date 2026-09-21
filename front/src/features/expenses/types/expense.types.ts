// GET /api/expenses/{expense_id} の通信 DTO。フィールド名は API の snake_case に合わせる

export type ExpenseSource = 'AI' | 'USER'

export type Expense = {
  expense_id: string
  store_name: string
  purchase_date: string
  read_amount: number
  adjustment_amount: number
  recorded_amount: number
  source: ExpenseSource
  is_edited: boolean
  updated_at: string
}

export type ExpenseDetail = {
  detail_id: string
  name: string
  category: string
  amount: number
  quantity: number
  source: ExpenseSource
  is_edited: boolean
}
