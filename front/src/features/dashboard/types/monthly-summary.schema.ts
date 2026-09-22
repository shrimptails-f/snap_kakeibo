import { z } from 'zod'

export const dashboardCategories = [
  'food',
  'daily_goods',
  'medical',
  'transport',
  'utilities',
  'entertainment',
  'social',
  'clothing',
  'education',
  'other',
  'unknown',
] as const

export const dashboardCategorySchema = z.enum(dashboardCategories)

export const monthlySummarySchema = z.object({
  year_month: z.string().regex(/^\d{4}-(0[1-9]|1[0-2])$/),
  total_recorded_amount: z.number().int(),
  expense_count: z.number().int().nonnegative(),
  detail_count: z.number().int().nonnegative(),
  category_totals: z.record(dashboardCategorySchema, z.number().int()),
  updated_at: z.string(),
})

export const getMonthlySummariesResponseSchema = z.object({
  monthly_summaries: z.array(monthlySummarySchema),
})
