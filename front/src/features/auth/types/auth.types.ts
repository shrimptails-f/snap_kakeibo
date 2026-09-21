import type { AuthSessionResponse } from '@/shared/auth/token'

export type AuthUser = {
  user_id: string
  email: string
}

// POST /api/auth/login
export type LoginRequest = {
  email: string
  password: string
}

export type LoginResponse = AuthSessionResponse & {
  user: AuthUser
}

// GET /api/auth/check
export type CheckAuthResponse = {
  user: AuthUser
}
