// バックエンド API 向けの共通 HTTP client。
// JSON の読み書き、Bearer access token の付与、401 時の refresh と再送、共通エラー変換をここに集約する。
// Presigned URL への PUT は送信先も認証方式も異なるため、この client を使わない。

export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'

export type QueryParams = Record<string, string | number | boolean | undefined | null>

export type RequestOptions<TBody = unknown> = {
  body?: TBody
  headers?: Record<string, string>
  query?: QueryParams
  signal?: AbortSignal
  credentials?: RequestCredentials
  // false にすると Authorization ヘッダーを付けない(login / refresh など)
  attachAuthToken?: boolean
  // false にすると 401 でも refresh と再送をしない。既定は attachAuthToken に従う
  retryOnUnauthorized?: boolean
}

type ApiErrorParams = {
  status: number
  apiMessage?: string
  body?: unknown
  cause?: unknown
}

// HTTP ステータスが 2xx 以外だったときに投げる。
// apiMessage はバックエンドの {"error": "..."} で、調査用の短い英文。利用者向け文言は shared/api/errors.ts で組み立てる
export class ApiError extends Error {
  readonly status: number
  readonly apiMessage: string | undefined
  readonly body: unknown

  constructor({ status, apiMessage, body, cause }: ApiErrorParams) {
    super(apiMessage ? `API error ${status}: ${apiMessage}` : `API error ${status}`, { cause })
    this.name = 'ApiError'
    this.status = status
    this.apiMessage = apiMessage
    this.body = body
  }
}

export function isApiError(error: unknown): error is ApiError {
  return error instanceof ApiError
}

export type ClientAuthConfig = {
  // この endpoint 自身が 401 を返しても refresh を試みない
  refreshEndpoint: string
  getAuthorizationHeaderValue: () => string | undefined
  hasAuthToken: () => boolean
  // refresh token(Cookie)で access token を取り直し、成功したら true を返す
  refreshAuthSession: () => Promise<boolean>
}

export type ClientConfig = {
  baseUrl: string
  defaultCredentials?: RequestCredentials
  auth?: ClientAuthConfig
}

type ExecuteRequestOptions<TBody> = RequestOptions<TBody> & {
  isRetryAfterUnauthorized?: boolean
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

// バックエンドは apigateway.Error で {"error": "message"} を返す。
// 将来 {"error": {"message": "..."}} や {"message": "..."} になっても読めるようにしておく
function readApiMessage(body: unknown): string | undefined {
  if (!isRecord(body)) return undefined
  if (typeof body.error === 'string') return body.error
  const candidate = isRecord(body.error) ? body.error : body
  return typeof candidate.message === 'string' ? candidate.message : undefined
}

function buildQueryString(query?: QueryParams): string {
  if (!query) return ''
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === null) continue
    params.append(key, String(value))
  }
  const queryString = params.toString()
  return queryString ? `?${queryString}` : ''
}

// 本文を JSON / 文字列 / なし(undefined)のいずれかに読む。JSON として壊れていれば ApiError にする
async function parseResponseBody(response: Response): Promise<unknown> {
  if (response.status === 204 || response.status === 304) return undefined

  const rawBody = await response.text()
  if (rawBody.trim() === '') return undefined

  const contentType = response.headers.get('Content-Type') ?? ''
  if (!contentType.includes('application/json')) return rawBody

  try {
    return JSON.parse(rawBody)
  } catch (cause: unknown) {
    throw new ApiError({ status: response.status, body: rawBody, cause })
  }
}

export class Client {
  private readonly config: ClientConfig
  // 同時に複数の 401 が起きても refresh は 1 回にまとめる
  private refreshRequest: Promise<boolean> | null = null

  constructor(config: ClientConfig) {
    this.config = config
  }

  request<TResponse, TBody = unknown>(
    method: HttpMethod,
    endpoint: string,
    options: RequestOptions<TBody> = {},
  ): Promise<TResponse> {
    return this.executeRequest<TResponse, TBody>(method, endpoint, options)
  }

  private buildUrl(endpoint: string, query?: QueryParams): string {
    const queryString = buildQueryString(query)
    if (/^https?:\/\//i.test(endpoint)) return `${endpoint}${queryString}`

    const baseUrl = this.config.baseUrl.replace(/\/+$/, '')
    const path = endpoint.replace(/^\/+/, '')
    return `${baseUrl}/${path}${queryString}`
  }

  private buildHeaders<TBody>(method: HttpMethod, options: RequestOptions<TBody>): Record<string, string> {
    const hasBody = method !== 'GET' && options.body !== undefined && options.body !== null
    const authorization =
      options.attachAuthToken === false ? undefined : this.config.auth?.getAuthorizationHeaderValue()

    return {
      ...(hasBody ? { 'Content-Type': 'application/json' } : {}),
      ...options.headers,
      ...(authorization ? { Authorization: authorization } : {}),
    }
  }

  private refreshAuthSession(): Promise<boolean> {
    const auth = this.config.auth
    if (!auth) return Promise.resolve(false)

    if (!this.refreshRequest) {
      this.refreshRequest = auth.refreshAuthSession().finally(() => {
        this.refreshRequest = null
      })
    }
    return this.refreshRequest
  }

  private async executeRequest<TResponse, TBody>(
    method: HttpMethod,
    endpoint: string,
    options: ExecuteRequestOptions<TBody>,
  ): Promise<TResponse> {
    const {
      body,
      query,
      signal,
      credentials = this.config.defaultCredentials ?? 'include',
      attachAuthToken = true,
      retryOnUnauthorized = attachAuthToken,
      isRetryAfterUnauthorized = false,
    } = options

    const hasBody = method !== 'GET' && body !== undefined && body !== null
    const response = await fetch(this.buildUrl(endpoint, query), {
      method,
      headers: this.buildHeaders(method, options),
      body: hasBody ? JSON.stringify(body) : undefined,
      signal,
      credentials,
    })

    const responseBody = await parseResponseBody(response)

    if (response.ok) {
      // 形式の検証は呼び出し側(feature の api)が行う
      return responseBody as TResponse
    }

    const shouldRetry =
      response.status === 401 &&
      retryOnUnauthorized &&
      !isRetryAfterUnauthorized &&
      endpoint !== this.config.auth?.refreshEndpoint

    if (shouldRetry) {
      const isRefreshed = await this.refreshAuthSession()
      if (isRefreshed && this.config.auth?.hasAuthToken()) {
        return this.executeRequest<TResponse, TBody>(method, endpoint, { ...options, isRetryAfterUnauthorized: true })
      }
    }

    throw new ApiError({ status: response.status, apiMessage: readApiMessage(responseBody), body: responseBody })
  }
}
