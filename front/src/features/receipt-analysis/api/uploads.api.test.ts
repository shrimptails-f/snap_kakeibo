import { afterEach, expect, it, vi } from 'vitest'
import { uploadToPresignedPost } from './uploads.api'

afterEach(() => vi.unstubAllGlobals())

it('署名済みフィールドの後に画像を追加して POST する', async () => {
  const fetchMock = vi.fn(async (_url: string, _init: RequestInit) => new Response(null, { status: 204 }))
  vi.stubGlobal('fetch', fetchMock)
  const file = new File(['image'], 'receipt.jpg', { type: 'image/jpeg' })

  await uploadToPresignedPost('https://s3.example.com/bucket', { key: 'receipts/u1/r1/original.jpg', policy: 'signed', 'Content-Type': 'image/jpeg' }, file)

  const [url, init] = fetchMock.mock.calls[0]
  expect(url).toBe('https://s3.example.com/bucket')
  expect(init.method).toBe('POST')
  expect(init.headers).toBeUndefined()
  const body = init.body as FormData
  expect([...body.keys()]).toEqual(['key', 'policy', 'Content-Type', 'file'])
  expect(body.get('file')).toBe(file)
})
