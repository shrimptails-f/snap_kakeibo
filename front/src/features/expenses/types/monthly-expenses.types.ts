import type { z } from 'zod'
import type { categorySchema, getMonthlySummariesResponseSchema, listMonthExpensesResponseSchema, monthlyExpenseItemSchema, monthlySummarySchema } from './monthly-expenses.schema'

export type Category = z.infer<typeof categorySchema>
export type MonthlySummary = z.infer<typeof monthlySummarySchema>
export type MonthlyExpenseItem = z.infer<typeof monthlyExpenseItemSchema>
export type GetMonthlySummariesResponse = z.infer<typeof getMonthlySummariesResponseSchema>
export type ListMonthExpensesResponse = z.infer<typeof listMonthExpensesResponseSchema>
