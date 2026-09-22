import type { Category, MonthlyExpenseItem } from '../types/monthly-expenses.types'

export const CATEGORY_ORDER: Category[] = ['food', 'daily_goods', 'medical', 'transport', 'utilities', 'entertainment', 'social', 'clothing', 'education', 'other', 'unknown']

export type CategoryBreakdown = { category: Category; amount: number; percentage: number | null }

export function monthlyBreakdown(items: MonthlyExpenseItem[]): { total: number; rows: CategoryBreakdown[]; canDrawChart: boolean } {
  const totals = new Map<Category, number>()
  for (const item of items) totals.set(item.category, (totals.get(item.category) ?? 0) + item.amount)
  const total = items.reduce((sum, item) => sum + item.amount, 0)
  const canDrawChart = total > 0 && items.every((item) => item.amount >= 0)
  return {
    total,
    canDrawChart,
    rows: CATEGORY_ORDER.filter((category) => totals.has(category)).map((category) => ({
      category, amount: totals.get(category) ?? 0, percentage: canDrawChart ? ((totals.get(category) ?? 0) / total) * 100 : null,
    })),
  }
}
