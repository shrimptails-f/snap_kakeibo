import { authSessionResponseSchema } from './auth-session.schema'
import type { AuthSessionResponse } from './auth-session.schema'

export type { AuthSessionResponse } from './auth-session.schema'

// access token はメモリにだけ保持し、localStorage / sessionStorage には書かない。
// 再読み込み後は Cookie の refresh token から取り直す(shared/auth/auth.api.ts)

const DEFAULT_TOKEN_TYPE = 'Bearer'

let memoryAccessToken: string | null = null
let memoryTokenType: string = DEFAULT_TOKEN_TYPE

// API レスポンスを境界で検証する
export function isAuthSessionResponse(value: unknown): value is AuthSessionResponse {
  return authSessionResponseSchema.safeParse(value).success
}

export function setAuthSession(session: AuthSessionResponse): void {
  memoryAccessToken = session.access_token
  memoryTokenType = session.token_type.trim() === '' ? DEFAULT_TOKEN_TYPE : session.token_type
}

export function clearAuthToken(): void {
  memoryAccessToken = null
  memoryTokenType = DEFAULT_TOKEN_TYPE
}

export function hasAuthToken(): boolean {
  return memoryAccessToken !== null
}

export function getAuthorizationHeaderValue(): string | undefined {
  if (memoryAccessToken === null) return undefined
  return `${memoryTokenType} ${memoryAccessToken}`
}
