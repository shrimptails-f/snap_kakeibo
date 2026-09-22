import { useQuery } from '@tanstack/react-query'
import { getMonthlySummaries, listMonthExpenses } from '../api/monthly-expenses.api'

export const monthlySummariesQueryKey = ['monthly-summaries'] as const
export const monthlyExpensesQueryKey = (yearMonth: string) => ['monthly-expenses', yearMonth] as const

export function useMonthlyExpenses(yearMonth: string, enabled = true) {
  const summaries = useQuery({ queryKey: monthlySummariesQueryKey, queryFn: ({ signal }) => getMonthlySummaries(signal), enabled })
  const expenses = useQuery({ queryKey: monthlyExpensesQueryKey(yearMonth), queryFn: ({ signal }) => listMonthExpenses(yearMonth, signal), enabled })
  return { summaries, expenses }
}
