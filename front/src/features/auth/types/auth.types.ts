import type { z } from 'zod'
import type { authUserSchema, checkAuthResponseSchema, loginResponseSchema } from './auth.schema'

export type AuthUser = z.infer<typeof authUserSchema>

// POST /api/auth/login
export type LoginRequest = {
  email: string
  password: string
}

export type LoginResponse = z.infer<typeof loginResponseSchema>

// GET /api/auth/check
export type CheckAuthResponse = z.infer<typeof checkAuthResponseSchema>
