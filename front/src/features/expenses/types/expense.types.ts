import type { z } from 'zod'
import type { expenseDetailSchema, expenseSchema, expenseSourceSchema, getExpenseResponseSchema, updateExpenseRequestSchema, updateExpenseResponseSchema } from './expense.schema'

// GET /api/expenses/{expense_id} の通信 DTO。形の定義は expense.schema.ts

export type ExpenseSource = z.infer<typeof expenseSourceSchema>

export type Expense = z.infer<typeof expenseSchema>

export type ExpenseDetail = z.infer<typeof expenseDetailSchema>

export type GetExpenseResponse = z.infer<typeof getExpenseResponseSchema>
export type UpdateExpenseRequest = z.infer<typeof updateExpenseRequestSchema>
export type UpdateExpenseResponse = z.infer<typeof updateExpenseResponseSchema>
