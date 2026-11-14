import { useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { createUpload, uploadToPresignedPost } from '../api/uploads.api'
import { validateReceiptFile } from '../lib/receiptCrop'
import type { CropRect } from '../lib/receiptCrop'
import { analysisRequestsQueryPrefix } from './useAnalysisRequests'

export type SelectedReceipt = {
  localId: string
  file: File
  originalFile: File
  originalUrl: string
  crop?: CropRect
  previewUrl: string
  validationError?: string
}

export type ReceiptUpload = SelectedReceipt & {
  analysisRequestId?: string
  expiresAt?: string
  phase: 'creating' | 'uploading' | 'sent' | 'failed'
  errorMessage?: string
}

function selectedReceipt(file: File): SelectedReceipt {
  const previewUrl = URL.createObjectURL(file)
  return {
    localId: crypto.randomUUID(), file, originalFile: file, originalUrl: previewUrl,
    previewUrl, validationError: validateReceiptFile(file),
  }
}

export function useReceiptUploadBatch(yearMonth: string) {
  const queryClient = useQueryClient()
  const [selected, setSelected] = useState<SelectedReceipt[]>([])
  const [uploads, setUploads] = useState<ReceiptUpload[]>([])
  const [isUploading, setIsUploading] = useState(false)
  const previews = useRef(new Set<string>())

  useEffect(() => () => previews.current.forEach((url) => URL.revokeObjectURL(url)), [])

  function addFiles(files: FileList | File[]) {
    const added = Array.from(files).map(selectedReceipt)
    added.forEach((item) => previews.current.add(item.previewUrl))
    setSelected((current) => [...current, ...added])
  }

  function releasePreview(url: string) {
    URL.revokeObjectURL(url)
    previews.current.delete(url)
  }

  function removeSelected(localId: string) {
    const removed = selected.find((item) => item.localId === localId)
    if (!removed || isUploading) return
    new Set([removed.previewUrl, removed.originalUrl]).forEach(releasePreview)
    setSelected((current) => current.filter((item) => item.localId !== localId))
  }

  function applyCrop(localId: string, file: File, crop?: CropRect) {
    const original = selected.find((item) => item.localId === localId)
    if (!original || isUploading || validateReceiptFile(file)) return
    const previewUrl = file === original.originalFile ? original.originalUrl : URL.createObjectURL(file)
    previews.current.add(previewUrl)
    if (original.previewUrl !== original.originalUrl) releasePreview(original.previewUrl)
    setSelected((current) => current.map((item) => item.localId === localId
      ? { ...item, file, previewUrl, crop, validationError: undefined } : item))
  }

  function updateUpload(localId: string, update: Partial<ReceiptUpload>) {
    setUploads((current) => current.map((item) => (item.localId === localId ? { ...item, ...update } : item)))
  }

  async function uploadOne(item: SelectedReceipt) {
    let hasCreatedRequest = false
    try {
      const created = await createUpload({ file_name: item.file.name, content_type: item.file.type })
      hasCreatedRequest = true
      updateUpload(item.localId, {
        analysisRequestId: created.analysis_request_id,
        expiresAt: created.expires_at,
        phase: 'uploading',
      })
      await uploadToPresignedPost(created.post_url, created.post_fields, item.file)
      updateUpload(item.localId, { phase: 'sent' })
    } catch {
      updateUpload(item.localId, {
        phase: 'failed',
        errorMessage: hasCreatedRequest ? '画像を送信できませんでした。' : '送信を開始できませんでした。',
      })
    }
  }

  async function retryCreating(localId: string) {
    const item = uploads.find((candidate) => candidate.localId === localId && !candidate.analysisRequestId)
    if (!item || isUploading) return
    setIsUploading(true)
    updateUpload(localId, { phase: 'creating', errorMessage: undefined })
    await uploadOne(item)
    await queryClient.invalidateQueries({ queryKey: analysisRequestsQueryPrefix(yearMonth) })
    setIsUploading(false)
  }

  async function uploadSelected() {
    const valid = selected.filter((item) => !item.validationError)
    if (valid.length === 0 || isUploading) return
    setIsUploading(true)
    setSelected((current) => current.filter((item) => item.validationError))
    valid.forEach((item) => { if (item.originalUrl !== item.previewUrl) releasePreview(item.originalUrl) })
    setUploads((current) => [...current, ...valid.map((item): ReceiptUpload => ({ ...item, originalFile: item.file, originalUrl: item.previewUrl, phase: 'creating' }))])
    await Promise.allSettled(valid.map(uploadOne))
    await queryClient.invalidateQueries({ queryKey: analysisRequestsQueryPrefix(yearMonth) })
    setIsUploading(false)
  }

  return {
    selected,
    uploads,
    isUploading,
    validCount: selected.filter((item) => !item.validationError).length,
    addFiles,
    removeSelected,
    applyCrop,
    retryCreating,
    uploadSelected,
  }
}
