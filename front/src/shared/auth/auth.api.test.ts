import { afterEach, describe, expect, it, vi } from 'vitest'
import { refreshAuthSession } from './auth.api'
import { clearAuthToken, getAuthorizationHeaderValue, hasAuthToken, setAuthSession } from './token'

function mockFetch(response: Response | Error) {
  const fetchMock = vi.fn(async () => {
    if (response instanceof Error) throw response
    return response
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

describe('refreshAuthSession', () => {
  afterEach(() => {
    clearAuthToken()
    vi.unstubAllGlobals()
  })

  it('Cookie だけで refresh を呼び、新しい access token をメモリへ保存する', async () => {
    const fetchMock = mockFetch(Response.json({ access_token: 'new-token', token_type: 'Bearer', expires_in: 900 }))
    setAuthSession({ access_token: 'old-token', token_type: 'Bearer', expires_in: 900 })

    await expect(refreshAuthSession()).resolves.toBe(true)

    expect(getAuthorizationHeaderValue()).toBe('Bearer new-token')
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/auth/refresh',
      expect.objectContaining({ method: 'POST', credentials: 'include', headers: {} }),
    )
  })

  it('401 なら token を消して false を返す', async () => {
    mockFetch(Response.json({ error: 'unauthorized' }, { status: 401 }))
    setAuthSession({ access_token: 'old-token', token_type: 'Bearer', expires_in: 900 })

    await expect(refreshAuthSession()).resolves.toBe(false)

    expect(hasAuthToken()).toBe(false)
  })

  it('応答が session の形式でなければ token を消して false を返す', async () => {
    mockFetch(Response.json({ token_type: 'Bearer' }))
    setAuthSession({ access_token: 'old-token', token_type: 'Bearer', expires_in: 900 })

    await expect(refreshAuthSession()).resolves.toBe(false)

    expect(hasAuthToken()).toBe(false)
  })

  it('通信に失敗したら token を消して false を返す', async () => {
    mockFetch(new TypeError('Failed to fetch'))
    setAuthSession({ access_token: 'old-token', token_type: 'Bearer', expires_in: 900 })

    await expect(refreshAuthSession()).resolves.toBe(false)

    expect(hasAuthToken()).toBe(false)
  })
})
