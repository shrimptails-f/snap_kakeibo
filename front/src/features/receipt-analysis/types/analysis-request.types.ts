import type { z } from 'zod'
import type {
  analysisRequestItemSchema,
  analysisRequestStatusSchema,
  createUploadResponseSchema,
  listAnalysisRequestsResponseSchema,
} from './analysis-request.schema'

// receipt-analysis の通信 DTO。形の定義は analysis-request.schema.ts

export type AnalysisRequestStatus = z.infer<typeof analysisRequestStatusSchema>

export type AnalysisRequestItem = z.infer<typeof analysisRequestItemSchema>

export type ListAnalysisRequestsResponse = z.infer<typeof listAnalysisRequestsResponseSchema>

export type CreateUploadRequest = {
  file_name: string
  content_type: string
}

export type CreateUploadResponse = z.infer<typeof createUploadResponseSchema>
