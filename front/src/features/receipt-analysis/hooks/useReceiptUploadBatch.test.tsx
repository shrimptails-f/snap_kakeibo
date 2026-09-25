import { act, renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { useReceiptUploadBatch } from './useReceiptUploadBatch'
import { createUpload, uploadToPresignedPost } from '../api/uploads.api'

vi.mock('../api/uploads.api', () => ({ createUpload: vi.fn(), uploadToPresignedPost: vi.fn() }))
beforeEach(() => {
  let count = 0
  vi.stubGlobal('URL', { createObjectURL: vi.fn(() => `blob:${++count}`), revokeObjectURL: vi.fn() })
  vi.mocked(createUpload).mockResolvedValue({ analysis_request_id: 'r1', post_url: 'https://example.com', post_fields: {}, expires_at: '2099-01-01T00:00:00Z' })
  vi.mocked(uploadToPresignedPost).mockResolvedValue(undefined)
})
afterEach(() => { vi.unstubAllGlobals(); vi.resetAllMocks() })

it('画像ごとに確定Fileを保持し、プレビュー・送信・再試行を一致させ、元画像URLも解放する', async () => {
  const client = new QueryClient()
  const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={client}>{children}</QueryClientProvider>
  const { result, unmount } = renderHook(() => useReceiptUploadBatch('2026-09'), { wrapper })
  const first = new File(['original'], 'same.jpg', { type: 'image/jpeg' })
  const second = new File(['second'], 'same.jpg', { type: 'image/jpeg' })
  const cropped = new File(['cropped'], 'same.jpg', { type: 'image/jpeg' })
  act(() => result.current.addFiles([first, second]))
  const [one, two] = result.current.selected
  act(() => result.current.applyCrop(one.localId, cropped, { x: 0.1, y: 0.1, width: 0.8, height: 0.8 }))
  expect(result.current.selected[0].originalFile).toBe(first)
  expect(result.current.selected[0].file).toBe(cropped)
  expect(result.current.selected[1].file).toBe(second)
  expect(URL.createObjectURL).toHaveBeenLastCalledWith(cropped)
  vi.mocked(createUpload).mockRejectedValueOnce(new Error('network'))
  await act(() => result.current.uploadSelected())
  expect(uploadToPresignedPost).toHaveBeenCalledWith('https://example.com', {}, second)
  await act(() => result.current.retryCreating(one.localId))
  expect(uploadToPresignedPost).toHaveBeenLastCalledWith('https://example.com', {}, cropped)
  expect(URL.revokeObjectURL).toHaveBeenCalledWith(one.originalUrl)
  unmount()
  expect(URL.revokeObjectURL).toHaveBeenCalledWith(two.originalUrl)
  expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:3')
})

it('元に戻すと元Fileを使い、除外後の編集結果を受け入れない', async () => {
  const client = new QueryClient()
  const { result, unmount } = renderHook(() => useReceiptUploadBatch('2026-09'), { wrapper: ({ children }) => <QueryClientProvider client={client}>{children}</QueryClientProvider> })
  const file = new File(['original'], 'a.png', { type: 'image/png' })
  const cropped = new File(['crop'], 'a.png', { type: 'image/png' })
  act(() => result.current.addFiles([file]))
  const original = result.current.selected[0]
  act(() => result.current.applyCrop(original.localId, cropped))
  act(() => result.current.applyCrop(original.localId, file))
  expect(result.current.selected[0].previewUrl).toBe(original.previewUrl)
  expect(result.current.selected[0].file).toBe(file)
  expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:2')
  act(() => result.current.removeSelected(original.localId))
  act(() => result.current.applyCrop(original.localId, cropped))
  await waitFor(() => expect(result.current.selected).toHaveLength(0))
  expect(URL.createObjectURL).toHaveBeenCalledTimes(2)
  unmount()
})
