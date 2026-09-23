import { dashboardCategories } from '../types/monthly-summary.schema'
import type { DashboardCategory, MonthlySummary } from '../types/monthly-summary.types'

export const CATEGORY_LABELS: Record<DashboardCategory, string> = {
  food: '食費',
  daily_goods: '日用品',
  medical: '医療',
  transport: '交通',
  utilities: '水道・光熱・通信',
  entertainment: '娯楽',
  social: '交際・会食',
  clothing: '衣類',
  education: '教育',
  other: 'その他',
  unknown: '分類不能',
}

export function currentYearMonth(now = new Date()): string {
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}`
}

export function shiftYearMonth(value: string, offset: number): string {
  const [year, month] = value.split('-').map(Number)
  const shifted = new Date(Date.UTC(year, month - 1 + offset, 1))
  return `${String(shifted.getUTCFullYear()).padStart(4, '0')}-${String(shifted.getUTCMonth() + 1).padStart(2, '0')}`
}

export function monthsEndingAt(endMonth: string, count = 6): string[] {
  return Array.from({ length: count }, (_, index) => shiftYearMonth(endMonth, index - count + 1))
}

export function monthsBetween(first: string, last: string): string[] {
  const months: string[] = []
  for (let month = first; month <= last; month = shiftYearMonth(month, 1)) months.push(month)
  return months
}

export function yearMonthLabel(value: string): string {
  const [year, month] = value.split('-').map(Number)
  return `${year}年${month}月`
}

export function shortMonthLabel(value: string, previous?: string): string {
  const [year, month] = value.split('-').map(Number)
  return !previous || previous.slice(0, 4) !== value.slice(0, 4) ? `${year}年${month}月` : `${month}月`
}

export function categoryRows(summary: MonthlySummary) {
  return dashboardCategories
    .map((category, order) => ({ category, label: CATEGORY_LABELS[category], amount: summary.category_totals[category], order }))
    .filter((row) => row.amount !== 0)
    .sort((left, right) => right.amount - left.amount || left.order - right.order)
}

export function detailTotal(summary: MonthlySummary): number {
  return dashboardCategories.reduce((total, category) => total + summary.category_totals[category], 0)
}
