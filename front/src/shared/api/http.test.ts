import { afterEach, describe, expect, it, vi } from 'vitest'
import { clearAuthToken, getAuthorizationHeaderValue, setAuthSession } from '@/shared/auth/token'
import { http, onUnauthorized } from './http'

// パスごとに応答を返し、呼び出し内容を残す
function mockFetch(routes: Record<string, () => Response>) {
  const calls: Array<{ url: string; init: RequestInit }> = []
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
    calls.push({ url, init: init ?? {} })
    const route = routes[url]
    if (route === undefined) return new Response(null, { status: 404 })
    return route()
  })
  vi.stubGlobal('fetch', fetchMock)
  return calls
}

describe('http', () => {
  afterEach(() => {
    clearAuthToken()
    vi.unstubAllGlobals()
  })

  it('401 のとき Cookie で refresh し、新しい token で再送する', async () => {
    let isRefreshed = false
    const calls = mockFetch({
      '/api/auth/check': () =>
        isRefreshed
          ? Response.json({ user: { user_id: 'u1', email: 'a@example.com' } })
          : Response.json({ error: 'unauthorized' }, { status: 401 }),
      '/api/auth/refresh': () => {
        isRefreshed = true
        return Response.json({ access_token: 'new-token', token_type: 'Bearer', expires_in: 900 })
      },
    })
    setAuthSession({ access_token: 'expired-token', token_type: 'Bearer', expires_in: 900 })

    await expect(http.get('/api/auth/check')).resolves.toEqual({ user: { user_id: 'u1', email: 'a@example.com' } })

    expect(calls.map((call) => call.url)).toEqual(['/api/auth/check', '/api/auth/refresh', '/api/auth/check'])
    expect(calls[1].init.headers).toEqual({})
    expect(calls[2].init.headers).toEqual({ Authorization: 'Bearer new-token' })
    expect(getAuthorizationHeaderValue()).toBe('Bearer new-token')
  })

  it('refresh も 401 なら token を消し、購読者へセッション切れを通知して ApiError を投げる', async () => {
    const calls = mockFetch({
      '/api/auth/check': () => Response.json({ error: 'unauthorized' }, { status: 401 }),
      '/api/auth/refresh': () => Response.json({ error: 'unauthorized' }, { status: 401 }),
    })
    setAuthSession({ access_token: 'expired-token', token_type: 'Bearer', expires_in: 900 })
    const listener = vi.fn()
    const unsubscribe = onUnauthorized(listener)

    await expect(http.get('/api/auth/check')).rejects.toMatchObject({ status: 401 })

    expect(calls.map((call) => call.url)).toEqual(['/api/auth/check', '/api/auth/refresh'])
    expect(getAuthorizationHeaderValue()).toBeUndefined()
    expect(listener).toHaveBeenCalledTimes(1)

    unsubscribe()
    setAuthSession({ access_token: 'expired-token', token_type: 'Bearer', expires_in: 900 })
    await expect(http.get('/api/auth/check')).rejects.toMatchObject({ status: 401 })
    expect(listener).toHaveBeenCalledTimes(1)
  })
})
