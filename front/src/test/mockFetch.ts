import { vi } from 'vitest'

export type FetchCall = { url: string; init: RequestInit }

type RouteHandler = (call: FetchCall) => Response

// パスごとの応答。値が関数ならリクエスト内容を見て Response を組み立てる。それ以外は 200 の JSON として返す
export type FetchRoutes = Record<string, RouteHandler | Record<string, unknown> | unknown[]>

// fetch を差し替えて、パス(query を除く)ごとの応答を返す。未定義のパスは 404
export function mockFetch(routes: FetchRoutes) {
  const calls: FetchCall[] = []
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
    const call = { url, init: init ?? {} }
    calls.push(call)
    const path = url.split('?')[0]
    if (!Object.hasOwn(routes, path)) {
      return new Response(null, { status: 404, statusText: 'Not Found' })
    }
    const route = routes[path]
    return typeof route === 'function' ? route(call) : Response.json(route)
  })
  vi.stubGlobal('fetch', fetchMock)
  return { fetchMock, calls }
}

export function jsonResponse(body: unknown, status = 200): Response {
  return Response.json(body, { status })
}

export function readAuthorization(init: RequestInit): string | undefined {
  return new Headers(init.headers).get('Authorization') ?? undefined
}
