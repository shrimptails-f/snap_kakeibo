import { afterEach, describe, expect, it, vi } from 'vitest'
import { ApiError, Client, type ClientAuthConfig } from './client'

type FetchCall = { url: string; init: RequestInit }

// fetch を差し替えて、呼び出し順に応答を返す。呼び出し内容は calls に残す
function mockFetch(responses: Array<Response | (() => Response)>) {
  const calls: FetchCall[] = []
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
    calls.push({ url, init: init ?? {} })
    const next = responses.shift()
    if (next === undefined) throw new Error(`unexpected fetch: ${url}`)
    return typeof next === 'function' ? next() : next
  })
  vi.stubGlobal('fetch', fetchMock)
  return { fetchMock, calls }
}

function jsonResponse(body: unknown, status = 200) {
  return Response.json(body, { status })
}

// 認証設定の mock。token は文字列を持つと Authorization を返す
function mockAuth(initialToken: string | undefined, refreshResult: boolean | (() => Promise<boolean>) = true) {
  let token = initialToken
  const refreshAuthSession = vi.fn(async () => {
    const isRefreshed = typeof refreshResult === 'function' ? await refreshResult() : refreshResult
    token = isRefreshed ? 'refreshed-token' : undefined
    return isRefreshed
  })
  const auth: ClientAuthConfig = {
    refreshEndpoint: '/api/auth/refresh',
    getAuthorizationHeaderValue: () => (token ? `Bearer ${token}` : undefined),
    hasAuthToken: () => token !== undefined,
    refreshAuthSession,
  }
  return { auth, refreshAuthSession }
}

describe('Client', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('baseUrl と endpoint を結合し、query の undefined / null は送らない', async () => {
    const { calls } = mockFetch([jsonResponse({ ok: true })])
    const client = new Client({ baseUrl: 'https://api.example.com/' })

    await client.request('GET', '/api/months', { query: { month: '2026-09', page: 2, cursor: undefined, empty: null } })

    expect(calls[0].url).toBe('https://api.example.com/api/months?month=2026-09&page=2')
    expect(calls[0].init.method).toBe('GET')
    expect(calls[0].init.credentials).toBe('include')
  })

  it('baseUrl が空なら同一 origin の相対パスで呼ぶ', async () => {
    const { calls } = mockFetch([jsonResponse({})])
    const client = new Client({ baseUrl: '' })

    await client.request('GET', '/api/auth/check')

    expect(calls[0].url).toBe('/api/auth/check')
  })

  it('body を JSON にして Content-Type と Authorization を付ける', async () => {
    const { calls } = mockFetch([jsonResponse({})])
    const { auth } = mockAuth('token-1')
    const client = new Client({ baseUrl: '', auth })

    await client.request('POST', '/api/uploads', { body: { file_name: 'a.jpg' } })

    expect(calls[0].init.body).toBe('{"file_name":"a.jpg"}')
    expect(calls[0].init.headers).toEqual({ 'Content-Type': 'application/json', Authorization: 'Bearer token-1' })
  })

  it('attachAuthToken: false なら Authorization を付けない', async () => {
    const { calls } = mockFetch([jsonResponse({})])
    const { auth } = mockAuth('token-1')
    const client = new Client({ baseUrl: '', auth })

    await client.request('POST', '/api/auth/login', { body: { email: 'a@example.com' }, attachAuthToken: false })

    expect(calls[0].init.headers).toEqual({ 'Content-Type': 'application/json' })
  })

  it('JSON 応答を返し、204 は undefined を返す', async () => {
    mockFetch([jsonResponse({ user: { user_id: 'u1' } }), new Response(null, { status: 204 })])
    const client = new Client({ baseUrl: '' })

    await expect(client.request('GET', '/api/auth/check')).resolves.toEqual({ user: { user_id: 'u1' } })
    await expect(client.request('POST', '/api/auth/logout')).resolves.toBeUndefined()
  })

  it('2xx 以外は {"error": "..."} を読んだ ApiError を投げる', async () => {
    mockFetch([jsonResponse({ error: 'too many login attempts' }, 429)])
    const client = new Client({ baseUrl: '' })

    const error = await client.request('POST', '/api/auth/login', { attachAuthToken: false }).catch((e: unknown) => e)

    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ status: 429, apiMessage: 'too many login attempts' })
  })

  it('JSON として壊れた応答は ApiError にする', async () => {
    mockFetch([new Response('<html>', { status: 200, headers: { 'Content-Type': 'application/json' } })])
    const client = new Client({ baseUrl: '' })

    const error = await client.request('GET', '/api/auth/check').catch((e: unknown) => e)

    expect(error).toBeInstanceOf(ApiError)
    expect(error).toMatchObject({ status: 200, body: '<html>' })
  })

  it('401 なら refresh してから新しい token で 1 回だけ再送する', async () => {
    const { calls } = mockFetch([jsonResponse({ error: 'unauthorized' }, 401), jsonResponse({ items: [] })])
    const { auth, refreshAuthSession } = mockAuth('expired-token')
    const client = new Client({ baseUrl: '', auth })

    await expect(client.request('GET', '/api/months/2026-09/analysis-requests')).resolves.toEqual({ items: [] })

    expect(refreshAuthSession).toHaveBeenCalledTimes(1)
    expect(calls).toHaveLength(2)
    expect(calls[0].init.headers).toEqual({ Authorization: 'Bearer expired-token' })
    expect(calls[1].init.headers).toEqual({ Authorization: 'Bearer refreshed-token' })
  })

  it('再送も 401 なら refresh を繰り返さず ApiError を投げる', async () => {
    mockFetch([jsonResponse({ error: 'unauthorized' }, 401), jsonResponse({ error: 'unauthorized' }, 401)])
    const { auth, refreshAuthSession } = mockAuth('expired-token')
    const client = new Client({ baseUrl: '', auth })

    await expect(client.request('GET', '/api/auth/check')).rejects.toMatchObject({ status: 401 })
    expect(refreshAuthSession).toHaveBeenCalledTimes(1)
  })

  it('refresh に失敗したら再送せず 401 の ApiError を投げる', async () => {
    const { calls } = mockFetch([jsonResponse({ error: 'unauthorized' }, 401)])
    const { auth } = mockAuth('expired-token', false)
    const client = new Client({ baseUrl: '', auth })

    await expect(client.request('GET', '/api/auth/check')).rejects.toMatchObject({ status: 401 })
    expect(calls).toHaveLength(1)
  })

  it('refresh endpoint 自身の 401 と retryOnUnauthorized: false では refresh しない', async () => {
    mockFetch([jsonResponse({ error: 'unauthorized' }, 401), jsonResponse({ error: 'unauthorized' }, 401)])
    const { auth, refreshAuthSession } = mockAuth('token-1')
    const client = new Client({ baseUrl: '', auth })

    await expect(client.request('POST', '/api/auth/refresh')).rejects.toMatchObject({ status: 401 })
    await expect(client.request('GET', '/api/auth/check', { retryOnUnauthorized: false })).rejects.toMatchObject({
      status: 401,
    })
    expect(refreshAuthSession).not.toHaveBeenCalled()
  })

  it('同時に複数の 401 が起きても refresh は 1 回にまとめる', async () => {
    const { calls } = mockFetch([
      jsonResponse({ error: 'unauthorized' }, 401),
      jsonResponse({ error: 'unauthorized' }, 401),
      jsonResponse({ a: 1 }),
      jsonResponse({ b: 2 }),
    ])
    let resolveRefresh: (value: boolean) => void = () => undefined
    const { auth, refreshAuthSession } = mockAuth(
      'expired-token',
      () =>
        new Promise<boolean>((resolve) => {
          resolveRefresh = resolve
        }),
    )
    const client = new Client({ baseUrl: '', auth })

    const first = client.request('GET', '/api/a')
    const second = client.request('GET', '/api/b')
    await vi.waitFor(() => expect(refreshAuthSession).toHaveBeenCalledTimes(1))
    resolveRefresh(true)

    await expect(Promise.all([first, second])).resolves.toEqual([{ a: 1 }, { b: 2 }])
    expect(refreshAuthSession).toHaveBeenCalledTimes(1)
    expect(calls.slice(2).map((call) => call.init.headers)).toEqual([
      { Authorization: 'Bearer refreshed-token' },
      { Authorization: 'Bearer refreshed-token' },
    ])
  })
})
