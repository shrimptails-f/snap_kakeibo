import { createContext, useContext } from 'react'
import type { AuthUser, LoginRequest } from '../types/auth.types'

export type AuthSessionStatus = 'checking' | 'authorized' | 'unauthorized'

export type AuthSession = {
  status: AuthSessionStatus
  user: AuthUser | null
  isChecking: boolean
  isAuthorized: boolean
  isUnauthorized: boolean
  // 失敗時は例外を投げる。文言は lib/loginErrorMessage で組み立てる
  login: (request: LoginRequest) => Promise<void>
  logout: () => Promise<void>
}

export const AuthSessionContext = createContext<AuthSession | null>(null)

// AuthSessionProvider の配下で、ログイン状態と login / logout を取得する
export function useAuthSession(): AuthSession {
  const session = useContext(AuthSessionContext)
  if (session === null) {
    throw new Error('useAuthSession は AuthSessionProvider の配下で使う')
  }
  return session
}
