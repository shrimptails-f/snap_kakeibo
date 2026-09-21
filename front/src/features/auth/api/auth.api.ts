import { http } from '@/shared/api/http'
import { clearAuthToken, isAuthSessionResponse, setAuthSession } from '@/shared/auth/token'
import type { AuthUser, CheckAuthResponse, LoginRequest, LoginResponse } from '../types/auth.types'

function isAuthUser(value: unknown): value is AuthUser {
  if (typeof value !== 'object' || value === null) return false
  const candidate = value as Record<string, unknown>
  return typeof candidate.user_id === 'string' && typeof candidate.email === 'string'
}

function isLoginResponse(value: unknown): value is LoginResponse {
  return isAuthSessionResponse(value) && isAuthUser((value as Record<string, unknown>).user)
}

function isCheckAuthResponse(value: unknown): value is CheckAuthResponse {
  return typeof value === 'object' && value !== null && isAuthUser((value as Record<string, unknown>).user)
}

// ログインし、access token をメモリへ保存する。refresh token は Set-Cookie でブラウザが保持する
export async function login(body: LoginRequest): Promise<LoginResponse> {
  const response = await http.post<unknown, LoginRequest>('/api/auth/login', {
    body,
    attachAuthToken: false,
  })
  if (!isLoginResponse(response)) {
    throw new Error('unexpected response shape: POST /api/auth/login')
  }
  setAuthSession(response)
  return response
}

// access token の有効性を確認し、ログイン中の利用者を返す。
// token が無い・失効している場合は apiClient が Cookie で refresh してから再送する(セッション復元)
export async function checkAuth(signal?: AbortSignal): Promise<CheckAuthResponse> {
  const response = await http.get<unknown>('/api/auth/check', { signal })
  if (!isCheckAuthResponse(response)) {
    throw new Error('unexpected response shape: GET /api/auth/check')
  }
  return response
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
