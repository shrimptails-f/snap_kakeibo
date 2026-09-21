import { z } from 'zod'

// receipt-analysis の API レスポンス。フィールド名は API の snake_case に合わせる。
// 型は analysis-request.types.ts が z.infer で導く

export const analysisRequestStatusSchema = z.enum(['UPLOADING', 'ANALYZING', 'SUCCEEDED', 'NO_DATA', 'FAILED'])

export const analysisRequestItemSchema = z.object({
  analysis_request_id: z.string(),
  expense_id: z.string().optional(),
  status: analysisRequestStatusSchema,
  attempt: z.number(),
  file_name: z.string(),
  year_month: z.string(),
  upload_expires_at: z.string(),
  error_code: z.string().optional(),
  error_message: z.string().optional(),
  failed_at: z.string().optional(),
  created_at: z.string(),
  updated_at: z.string(),
})

// GET /api/months/{yyyy-MM}/analysis-requests
export const listAnalysisRequestsResponseSchema = z.object({
  items: z.array(analysisRequestItemSchema),
})

// POST /api/uploads
export const createUploadResponseSchema = z.object({
  put_url: z.string(),
  analysis_request_id: z.string(),
})
