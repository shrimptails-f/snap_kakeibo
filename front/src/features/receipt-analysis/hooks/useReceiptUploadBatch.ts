import { useEffect, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { createUpload, uploadToPresignedUrl } from '../api/uploads.api'
import { analysisRequestsQueryPrefix } from './useAnalysisRequests'

export type SelectedReceipt = {
  localId: string
  file: File
  previewUrl: string
  validationError?: string
}

export type ReceiptUpload = SelectedReceipt & {
  analysisRequestId?: string
  expiresAt?: string
  phase: 'creating' | 'uploading' | 'sent' | 'failed'
  errorMessage?: string
}

const ACCEPTED_IMAGE_TYPES = new Set(['image/jpeg', 'image/png'])

function selectedReceipt(file: File): SelectedReceipt {
  return {
    localId: crypto.randomUUID(),
    file,
    previewUrl: URL.createObjectURL(file),
    validationError: ACCEPTED_IMAGE_TYPES.has(file.type) ? undefined : 'JPEG または PNG の画像を選んでください。',
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

  function removeSelected(localId: string) {
    setSelected((current) => {
      const removed = current.find((item) => item.localId === localId)
      if (removed) {
        URL.revokeObjectURL(removed.previewUrl)
        previews.current.delete(removed.previewUrl)
      }
      return current.filter((item) => item.localId !== localId)
    })
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
      await uploadToPresignedUrl(created.put_url, item.file, item.file.type)
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
    setUploads((current) => [...current, ...valid.map((item): ReceiptUpload => ({ ...item, phase: 'creating' }))])
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
    retryCreating,
    uploadSelected,
  }
}
