import { http } from '@/shared/api/http'
import { parseResponse } from '@/shared/api/parseResponse'
import { refreshAuthSession } from '@/shared/auth/auth.api'
import { clearAuthToken, hasAuthToken, setAuthSession } from '@/shared/auth/token'
import { checkAuthResponseSchema, loginResponseSchema } from '../types/auth.schema'
import type { CheckAuthResponse, LoginRequest, LoginResponse } from '../types/auth.types'

// ログインし、access token をメモリへ保存する。refresh token は Set-Cookie でブラウザが保持する
export async function login(body: LoginRequest): Promise<LoginResponse> {
  const raw = await http.post<unknown, LoginRequest>('/api/auth/login', {
    body,
    attachAuthToken: false,
  })
  const response = parseResponse(loginResponseSchema, raw, 'POST /api/auth/login')
  setAuthSession(response)
  return response
}

// access token の有効性を確認し、ログイン中の利用者を返す。
// token が失効している場合は apiClient が Cookie で refresh してから再送する
export async function checkAuth(signal?: AbortSignal): Promise<CheckAuthResponse> {
  const raw = await http.get<unknown>('/api/auth/check', { signal })
  return parseResponse(checkAuthResponseSchema, raw, 'GET /api/auth/check')
}

// 起動時のセッション復元。メモリに token が無ければ先に Cookie で refresh してから check する。
// refresh token が無い・失効している場合は null(未ログイン)
export async function restoreAuthSession(signal?: AbortSignal): Promise<CheckAuthResponse | null> {
  if (!hasAuthToken() && !(await refreshAuthSession())) return null
  return checkAuth(signal)
}

// refresh token を失効させ、メモリの access token を消す
export async function logout(): Promise<void> {
  try {
    await http.post<undefined>('/api/auth/logout', { retryOnUnauthorized: false })
  } finally {
    // サーバー側の失効に失敗しても、この端末では未ログインに戻す
    clearAuthToken()
  }
}
