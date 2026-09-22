import { z } from 'zod'

// POST /api/auth/login と POST /api/auth/refresh が共通で返す部分。access_token が空なら session として扱わない
export const authSessionResponseSchema = z.object({
  access_token: z.string().refine((token) => token.trim() !== ''),
  token_type: z.string(),
  expires_in: z.number(),
})

export type AuthSessionResponse = z.infer<typeof authSessionResponseSchema>
