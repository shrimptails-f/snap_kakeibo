import { z } from 'zod'
import { expenseSourceSchema } from './expense.schema'

export const categorySchema = z.enum(['food', 'daily_goods', 'medical', 'transport', 'utilities', 'entertainment', 'social', 'clothing', 'education', 'other', 'unknown'])
const yearMonthSchema = z.string().regex(/^\d{4}-(0[1-9]|1[0-2])$/)
const purchaseDateSchema = z.string().regex(/^\d{4}-(0[1-9]|1[0-2])-([0-2]\d|3[01])$/)

export const monthlySummarySchema = z.object({
  year_month: yearMonthSchema,
  total_recorded_amount: z.number().int(),
  expense_count: z.number().int().nonnegative(),
  detail_count: z.number().int().nonnegative(),
  confirmed_detail_count: z.number().int().nonnegative().optional(),
  category_totals: z.record(categorySchema, z.number().int()),
  updated_at: z.string(),
})

export const getMonthlySummariesResponseSchema = z.object({ monthly_summaries: z.array(monthlySummarySchema) })

export const monthlyExpenseItemSchema = z.object({
  detail_id: z.string(), expense_id: z.string(), name: z.string(), category: categorySchema,
  amount: z.number().int(), quantity: z.number().int().positive(), source: expenseSourceSchema,
  tax_included_amount: z.number().int().optional(), tax_rate: z.number().int().optional(),
  tax_mode: z.enum(['included', 'external', 'mixed', 'unknown']).optional(),
  is_edited: z.boolean(), store_name: z.string(), purchase_date: purchaseDateSchema,
})

export const listMonthExpensesResponseSchema = z.object({ year_month: yearMonthSchema, items: z.array(monthlyExpenseItemSchema) })
