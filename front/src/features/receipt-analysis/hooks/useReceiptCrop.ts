import { useEffect, useRef, useState } from 'react'
import { cropPixels, FULL_CROP, validateReceiptFile } from '../lib/receiptCrop'
import type { CropRect } from '../lib/receiptCrop'

export function useReceiptCrop(file: File, originalUrl: string, initialCrop?: CropRect) {
  const [rect, setRect] = useState(initialCrop ?? FULL_CROP)
  const [size, setSize] = useState<{ width: number; height: number } | null>(null)
  const [message, setMessage] = useState('画像を読み込んでいます…')
  const [error, setError] = useState<string | null>(null)
  const [isSaving, setIsSaving] = useState(false)
  const source = useRef<HTMLImageElement | null>(null)
  const touched = useRef(false)
  const generation = useRef(0)
  const workerRef = useRef<Worker | null>(null)
  const saving = useRef(false)

  useEffect(() => {
    const session = generation
    const current = ++session.current
    const image = new Image()
    source.current = image
    let worker: Worker | undefined
    let timeout: ReturnType<typeof setTimeout> | undefined
    const fallback = () => {
      if (generation.current === current && !touched.current) setMessage('範囲を特定できませんでした。画像全体から調整できます。')
      worker?.terminate()
      clearTimeout(timeout)
    }
    image.onload = () => {
      if (generation.current !== current) return
      setSize({ width: image.naturalWidth, height: image.naturalHeight })
      if (initialCrop || touched.current) { setMessage('切り取り範囲を確認・調整してください。'); return }
      setMessage('レシートの範囲を探しています… 手動でも調整できます。')
      try {
        const scale = Math.min(1, 480 / Math.max(image.naturalWidth, image.naturalHeight))
        const canvas = document.createElement('canvas')
        canvas.width = Math.max(1, Math.round(image.naturalWidth * scale))
        canvas.height = Math.max(1, Math.round(image.naturalHeight * scale))
        const context = canvas.getContext('2d', { willReadFrequently: true })
        if (!context) throw new Error('canvas unavailable')
        context.drawImage(image, 0, 0, canvas.width, canvas.height)
        const pixels = context.getImageData(0, 0, canvas.width, canvas.height)
        canvas.width = canvas.height = 0
        worker = new Worker(new URL('../workers/receiptDetection.worker.ts', import.meta.url), { type: 'module' })
        workerRef.current = worker
        worker.onmessage = (event: MessageEvent<{ rect: CropRect | null }>) => {
          if (generation.current === current && !touched.current) {
            const proposal = event.data.rect
            setRect(proposal ?? FULL_CROP)
            setMessage(!proposal
              ? '範囲を特定できませんでした。画像全体から調整できます。'
              : proposal.y === 0 && proposal.height === 1
                ? '左右の余白を提案しました。上下は画像全体を残しています。商品や金額が入っているか確認してください。'
                : '範囲の候補です。店名・商品・日付・合計が入っているか確認してください。')
          }
          worker?.terminate()
          clearTimeout(timeout)
        }
        worker.onerror = fallback
        timeout = setTimeout(fallback, 5000)
        worker.postMessage({ data: pixels.data, width: pixels.width, height: pixels.height }, [pixels.data.buffer])
      } catch { fallback() }
    }
    image.onerror = () => {
      if (generation.current === current) setError('画像を開けませんでした。キャンセルして別の画像を選んでください。')
    }
    // HTMLImageElementはEXIFの向きを反映する。表示とCanvasの切り抜きで同じ画像を使う。
    image.src = originalUrl
    return () => {
      session.current++
      worker?.terminate()
      clearTimeout(timeout)
      image.onload = image.onerror = null
      image.src = ''
      source.current = null
    }
  }, [originalUrl, initialCrop])

  function changeRect(next: CropRect) {
    touched.current = true
    workerRef.current?.terminate()
    setRect(next)
    setMessage('店名・商品・日付・合計が入っているか確認してください。')
  }

  async function exportCrop(): Promise<{ file: File; crop?: CropRect } | null> {
    if (!source.current || !size || saving.current) return null
    const current = generation.current
    saving.current = true
    setIsSaving(true)
    setError(null)
    const canvas = document.createElement('canvas')
    try {
      if (rect.x === 0 && rect.y === 0 && rect.width === 1 && rect.height === 1) return { file }
      const pixels = cropPixels(rect, size.width, size.height)
      canvas.width = pixels.width
      canvas.height = pixels.height
      const context = canvas.getContext('2d')
      if (!context) throw new Error('canvas unavailable')
      context.drawImage(source.current, pixels.x, pixels.y, pixels.width, pixels.height, 0, 0, pixels.width, pixels.height)
      const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, file.type, 0.95))
      if (generation.current !== current) return null
      if (!blob) throw new Error('encoding failed')
      const result = new File([blob], file.name, { type: blob.type })
      const validationError = validateReceiptFile(result)
      if (validationError) { setError(validationError); return null }
      return { file: result, crop: rect }
    } catch {
      if (generation.current === current) setError('切り取り画像を作成できませんでした。範囲を小さくするか、切り取らずに使ってください。')
      return null
    } finally {
      canvas.width = canvas.height = 0
      saving.current = false
      if (generation.current === current) setIsSaving(false)
    }
  }

  return { rect, size, message, error, isSaving, changeRect, exportCrop }
}
