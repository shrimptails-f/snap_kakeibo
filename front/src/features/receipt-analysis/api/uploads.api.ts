import { http } from '@/shared/api/http'
import { parseResponse } from '@/shared/api/parseResponse'
import { createUploadResponseSchema } from '../types/analysis-request.schema'
import type { CreateUploadRequest, CreateUploadResponse } from '../types/analysis-request.types'

export async function createUpload(body: CreateUploadRequest): Promise<CreateUploadResponse> {
  const raw = await http.post<unknown, CreateUploadRequest>('/api/uploads', { body })
  return parseResponse(createUploadResponseSchema, raw, 'POST /api/uploads')
}

// S3 への POST は署名済みフィールドをそのまま送る。Content-Type はブラウザに multipart 境界を付けさせる。
export async function uploadToPresignedPost(postUrl: string, fields: Record<string, string>, file: File): Promise<void> {
  const body = new FormData()
  Object.entries(fields).forEach(([key, value]) => body.append(key, value))
  body.append('file', file)
  const response = await fetch(postUrl, {
    method: 'POST',
    body,
  })
  if (!response.ok) {
    throw new Error(`presigned upload failed: ${response.status}`)
  }
}
