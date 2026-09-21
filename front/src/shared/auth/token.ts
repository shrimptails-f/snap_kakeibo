// access token はメモリにだけ保持し、localStorage / sessionStorage には書かない。
// 再読み込み後は Cookie の refresh token から取り直す(shared/auth/auth.api.ts)

// POST /api/auth/login と POST /api/auth/refresh が共通で返す部分
export type AuthSessionResponse = {
  access_token: string
  token_type: string
  expires_in: number
}

const DEFAULT_TOKEN_TYPE = 'Bearer'

let memoryAccessToken: string | null = null
let memoryTokenType: string = DEFAULT_TOKEN_TYPE

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

// API レスポンスを境界で検証する。access_token が空なら session として扱わない
export function isAuthSessionResponse(value: unknown): value is AuthSessionResponse {
  return (
    isRecord(value) &&
    typeof value.access_token === 'string' &&
    value.access_token.trim() !== '' &&
    typeof value.token_type === 'string' &&
    typeof value.expires_in === 'number'
  )
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
