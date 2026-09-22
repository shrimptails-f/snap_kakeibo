import { z } from 'zod'

// GET /api/expenses/{expense_id} のレスポンス。フィールド名は API の snake_case に合わせる。
// 型は expense.types.ts が z.infer で導く

export const expenseSourceSchema = z.enum(['AI', 'USER'])

export const expenseSchema = z.object({
  expense_id: z.string(),
  analysis_request_id: z.string(),
  store_name: z.string(),
  purchase_date: z.string(),
  year_month: z.string(),
  read_amount: z.number(),
  adjustment_amount: z.number(),
  recorded_amount: z.number(),
  source: expenseSourceSchema,
  is_edited: z.boolean(),
  updated_at: z.string(),
})

export const expenseDetailSchema = z.object({
  detail_id: z.string(),
  name: z.string(),
  category: z.string(),
  category_source: expenseSourceSchema,
  amount: z.number(),
  quantity: z.number(),
  source: expenseSourceSchema,
  is_edited: z.boolean(),
})

export const getExpenseResponseSchema = z.object({
  expense: expenseSchema,
  details: z.array(expenseDetailSchema),
})

const integerInput = (minimum: number, maximum: number, message: string) =>
  z.number({ error: message }).int(message).min(minimum, message).max(maximum, message)

export const updateExpenseDetailSchema = z.object({
  detail_id: z.string(),
  name: z.string().trim().min(1, '商品名を入力してください。'),
  amount: integerInput(0, 10_000_000, '明細金額は0〜10,000,000の整数で入力してください。'),
  quantity: integerInput(1, 999, '数量は1〜999の整数で入力してください。'),
  category: z.enum(['food', 'daily_goods', 'medical', 'transport', 'utilities', 'entertainment', 'social', 'clothing', 'education', 'other', 'unknown']),
})

export const updateExpenseRequestSchema = z.object({
  store_name: z.string().trim().min(1, '店舗名を入力してください。'),
  purchase_date: z.string().refine((value) => {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false
    const date = new Date(`${value}T00:00:00Z`)
    return !Number.isNaN(date.getTime()) && date.toISOString().slice(0, 10) === value
  }, '実在する購入日を入力してください。'),
  adjustment_amount: z.number({ error: '調整額を整数で入力してください。' }).int('調整額を整数で入力してください。').safe(),
  details: z.array(updateExpenseDetailSchema).min(1),
})

export const updateExpenseResponseSchema = z.object({
  expense: z.object({
    expense_id: z.string(), read_amount: z.number(), adjustment_amount: z.number(), recorded_amount: z.number(), updated_at: z.string(),
  }),
})
