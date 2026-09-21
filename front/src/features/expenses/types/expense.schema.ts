import { z } from 'zod'

// GET /api/expenses/{expense_id} のレスポンス。フィールド名は API の snake_case に合わせる。
// 型は expense.types.ts が z.infer で導く

export const expenseSourceSchema = z.enum(['AI', 'USER'])

export const expenseSchema = z.object({
  expense_id: z.string(),
  store_name: z.string(),
  purchase_date: z.string(),
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
  amount: z.number(),
  quantity: z.number(),
  source: expenseSourceSchema,
  is_edited: z.boolean(),
})

export const getExpenseResponseSchema = z.object({
  expense: expenseSchema,
  details: z.array(expenseDetailSchema),
})
