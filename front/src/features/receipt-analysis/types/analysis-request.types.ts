// GET /api/months/{yyyy-MM}/analysis-requests の通信 DTO。フィールド名は API の snake_case に合わせる

export type AnalysisRequestStatus = 'UPLOADING' | 'ANALYZING' | 'SUCCEEDED' | 'NO_DATA' | 'FAILED'

export type AnalysisRequestItem = {
  analysis_request_id: string
  expense_id?: string
  status: AnalysisRequestStatus
  attempt: number
  file_name: string
  year_month: string
  upload_expires_at: string
  error_code?: string
  error_message?: string
  failed_at?: string
  created_at: string
  updated_at: string
}
