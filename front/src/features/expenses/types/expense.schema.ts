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
  // backend / frontend の独立デプロイ中も詳細表示を壊さないため、移行期間は未返却を許容する。
  image_url: z.string().url().optional(),
})

export const expenseDetailSchema = z.object({
  detail_id: z.string(),
  name: z.string(),
  category: z.string(),
  category_source: expenseSourceSchema,
  amount: z.number(),
  tax_included_amount: z.number().int().optional(),
  tax_rate: z.number().int().optional(),
  tax_mode: z.enum(['included', 'external', 'mixed', 'unknown']).optional(),
  tax_status: z.enum(['printed', 'reconciled', 'estimated', 'unresolved', 'user_confirmed']).optional(),
  tax_reason: z.string().optional(),
  suggested_tax_rate: z.number().int().optional(),
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
  detail_id: z.string().optional(),
  name: z.string().trim().min(1, '商品名を入力してください。'),
  amount: integerInput(0, 10_000_000, '明細金額は0〜10,000,000の整数で入力してください。'),
  quantity: integerInput(1, 999, '数量は1〜999の整数で入力してください。'),
  category: z.enum(['food', 'daily_goods', 'medical', 'transport', 'utilities', 'entertainment', 'social', 'clothing', 'education', 'other', 'unknown']),
  tax_mode: z.enum(['unknown', 'included', 'external']),
  tax_rate: z.union([z.literal(0), z.literal(8), z.literal(10)]),
  tax_included_amount: z.number().int().min(0).max(10_000_000).optional(),
  tax_confirmed: z.boolean().optional(),
}).superRefine((detail, context) => {
  if (detail.tax_mode === 'unknown') return
  if (detail.tax_rate === 0) context.addIssue({ code: 'custom', path: ['tax_rate'], message: '税率を選択してください。' })
  if (detail.tax_mode === 'external' && (detail.tax_included_amount === undefined || detail.tax_included_amount < detail.amount || detail.tax_included_amount > detail.amount + Math.ceil(detail.amount * detail.tax_rate / 100))) {
    context.addIssue({ code: 'custom', path: ['tax_included_amount'], message: '税込み額は印字額以上、税率から計算した上限以下で入力してください。' })
  }
})

export const updateExpenseRequestSchema = z.object({
  store_name: z.string().trim().min(1, '店舗名を入力してください。'),
  purchase_date: z.string().refine((value) => {
    if (!/^\d{4}-\d{2}-\d{2}$/.test(value)) return false
    const date = new Date(`${value}T00:00:00Z`)
    return !Number.isNaN(date.getTime()) && date.toISOString().slice(0, 10) === value
  }, '実在する購入日を入力してください。'),
  adjustment_amount: z.number({ error: '調整額を整数で入力してください。' }).int('調整額を整数で入力してください。').safe(),
  details: z.array(updateExpenseDetailSchema).min(1, '明細は1件以上必要です。').max(50, '明細は50件までです。'),
})

export const updateExpenseResponseSchema = z.object({
  expense: z.object({
    expense_id: z.string(), read_amount: z.number(), adjustment_amount: z.number(), recorded_amount: z.number(), updated_at: z.string(),
  }),
})
