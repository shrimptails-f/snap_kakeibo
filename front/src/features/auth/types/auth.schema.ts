import { z } from 'zod'
import { authSessionResponseSchema } from '@/shared/auth/auth-session.schema'

// 認証 API のレスポンス。型は auth.types.ts が z.infer で導く

export const authUserSchema = z.object({
  user_id: z.string(),
  email: z.string(),
})

// POST /api/auth/login
export const loginResponseSchema = authSessionResponseSchema.extend({
  user: authUserSchema,
})

// GET /api/auth/check
export const checkAuthResponseSchema = z.object({
  user: authUserSchema,
})
