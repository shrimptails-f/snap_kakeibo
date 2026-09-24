import { act, renderHook, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { useReceiptCrop } from './useReceiptCrop'
import { FULL_CROP } from '../lib/receiptCrop'

let image: { onload: (() => void) | null; onerror: (() => void) | null; src: string; naturalWidth: number; naturalHeight: number }
let worker: { onmessage: ((event: { data: { rect: typeof FULL_CROP | null } }) => void) | null; onerror: (() => void) | null; terminate: ReturnType<typeof vi.fn>; postMessage: ReturnType<typeof vi.fn> }
beforeEach(() => {
  vi.stubGlobal('Image', class { constructor() { image = { onload: null, onerror: null, src: '', naturalWidth: 3000, naturalHeight: 4000 }; return image } })
  vi.stubGlobal('Worker', class { constructor() { worker = { onmessage: null, onerror: null, terminate: vi.fn(), postMessage: vi.fn() }; return worker } })
  vi.spyOn(HTMLCanvasElement.prototype, 'getContext').mockReturnValue({ drawImage: vi.fn(), getImageData: () => ({ data: new Uint8ClampedArray(4), width: 1, height: 1 }) } as unknown as CanvasRenderingContext2D)
})
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })
const file = new File(['image'], 'receipt.jpg', { type: 'image/jpeg' })

it('検出結果は提案のみで、手動調整後の遅延結果を無視する', () => {
  const { result, unmount } = renderHook(() => useReceiptCrop(file, 'blob:original'))
  act(() => image.onload?.())
  const manual = { x: 0.1, y: 0.2, width: 0.8, height: 0.7 }
  act(() => result.current.changeRect(manual))
  act(() => worker.onmessage?.({ data: { rect: FULL_CROP } }))
  expect(result.current.rect).toEqual(manual)
  unmount()
  expect(worker.terminate).toHaveBeenCalled()
  expect(image.src).toBe('')
})

it('検出失敗でも全体を使い、キャンセル後の検出結果を無視する', () => {
  const { result, unmount } = renderHook(() => useReceiptCrop(file, 'blob:original'))
  act(() => image.onload?.())
  act(() => worker.onerror?.())
  expect(result.current.rect).toEqual(FULL_CROP)
  expect(result.current.message).toMatch(/特定できません/)
  unmount()
  act(() => worker.onmessage?.({ data: { rect: { x: 0.2, y: 0.2, width: 0.5, height: 0.5 } } }))
  expect(result.current.rect).toEqual(FULL_CROP)
})

it('元解像度から切り抜き、出力中に閉じた場合は結果を返さない', async () => {
  let finish: BlobCallback | undefined
  vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation((callback) => { finish = callback })
  const rect = { x: 0.1, y: 0.2, width: 0.5, height: 0.5 }
  const { result, unmount } = renderHook(() => useReceiptCrop(file, 'blob:original', rect))
  act(() => image.onload?.())
  let pending: ReturnType<typeof result.current.exportCrop>
  act(() => { pending = result.current.exportCrop() })
  expect(HTMLCanvasElement.prototype.getContext).toHaveLastReturnedWith(expect.objectContaining({ drawImage: expect.any(Function) }))
  const context = document.createElement('canvas').getContext('2d')!
  expect(context.drawImage).toHaveBeenCalledWith(image, 300, 800, 1500, 2000, 0, 0, 1500, 2000)
  unmount()
  finish?.(new Blob(['crop'], { type: 'image/jpeg' }))
  expect(await pending!).toBeNull()
})

it('出力失敗は元画像と範囲を保持して再操作できる', async () => {
  vi.spyOn(HTMLCanvasElement.prototype, 'toBlob').mockImplementation((callback) => callback(null))
  const rect = { x: 0.1, y: 0.2, width: 0.5, height: 0.5 }
  const { result } = renderHook(() => useReceiptCrop(file, 'blob:original', rect))
  act(() => image.onload?.())
  await act(() => result.current.exportCrop())
  await waitFor(() => expect(result.current.error).toMatch(/作成できません/))
  expect(result.current.rect).toEqual(rect)
  expect(result.current.isSaving).toBe(false)
})

it('検出が5秒を超えた場合はWorkerを終了し全体から手動調整できる', () => {
  vi.useFakeTimers()
  try {
    const { result, unmount } = renderHook(() => useReceiptCrop(file, 'blob:original'))
    act(() => image.onload?.())
    act(() => vi.advanceTimersByTime(5000))
    expect(result.current.rect).toEqual(FULL_CROP)
    expect(result.current.message).toMatch(/特定できません/)
    expect(worker.terminate).toHaveBeenCalled()
    unmount()
  } finally { vi.useRealTimers() }
})

it('Worker未対応でも全体を使え、全体の適用は元Fileを返す', async () => {
  vi.stubGlobal('Worker', undefined)
  const { result } = renderHook(() => useReceiptCrop(file, 'blob:original'))
  act(() => image.onload?.())
  expect(result.current.message).toMatch(/特定できません/)
  let exported: Awaited<ReturnType<typeof result.current.exportCrop>> = null
  await act(async () => { exported = await result.current.exportCrop() })
  expect(exported).toEqual({ file })
})
