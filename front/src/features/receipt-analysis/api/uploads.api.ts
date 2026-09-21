import { http } from '@/shared/api/http'

export type CreateUploadRequest = {
  file_name: string
  content_type: string
}

export type CreateUploadResponse = {
  put_url: string
  analysis_request_id: string
}

function isCreateUploadResponse(value: unknown): value is CreateUploadResponse {
  if (typeof value !== 'object' || value === null) return false
  const candidate = value as Record<string, unknown>
  return typeof candidate.put_url === 'string' && typeof candidate.analysis_request_id === 'string'
}

export async function createUpload(body: CreateUploadRequest): Promise<CreateUploadResponse> {
  const response = await http.post<unknown, CreateUploadRequest>('/api/uploads', { body })
  if (!isCreateUploadResponse(response)) {
    throw new Error('unexpected response shape: POST /api/uploads')
  }
  return response
}

// Presigned URL への PUT は送信先が S3 で認証も URL に含まれるため、apiClient を通さず素の fetch で送る
export async function uploadToPresignedUrl(putUrl: string, file: File, contentType: string): Promise<void> {
  const response = await fetch(putUrl, {
    method: 'PUT',
    headers: { 'content-type': contentType },
    body: file,
  })
  if (!response.ok) {
    throw new Error(`presigned upload failed: ${response.status}`)
  }
}
