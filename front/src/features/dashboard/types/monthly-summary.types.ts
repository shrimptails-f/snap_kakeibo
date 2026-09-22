import type { z } from 'zod'
import type {
  dashboardCategorySchema,
  getMonthlySummariesResponseSchema,
  monthlySummarySchema,
} from './monthly-summary.schema'

export type DashboardCategory = z.infer<typeof dashboardCategorySchema>
export type MonthlySummary = z.infer<typeof monthlySummarySchema>
export type GetMonthlySummariesResponse = z.infer<typeof getMonthlySummariesResponseSchema>
