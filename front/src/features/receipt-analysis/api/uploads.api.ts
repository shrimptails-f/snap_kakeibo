import { http } from '@/shared/api/http'
import { parseResponse } from '@/shared/api/parseResponse'
import { createUploadResponseSchema } from '../types/analysis-request.schema'
import type { CreateUploadRequest, CreateUploadResponse } from '../types/analysis-request.types'

export async function createUpload(body: CreateUploadRequest): Promise<CreateUploadResponse> {
  const raw = await http.post<unknown, CreateUploadRequest>('/api/uploads', { body })
  return parseResponse(createUploadResponseSchema, raw, 'POST /api/uploads')
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
