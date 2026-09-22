import type { z } from 'zod'

// API レスポンスを境界で検証する。形が違えば通信エラーと同じ扱い(toFriendlyMessage の既定文言)にし、
// 画面へは zod のメッセージを出さない。調査用に endpoint と issue を Error に残す
export function parseResponse<T>(schema: z.ZodType<T>, value: unknown, endpoint: string): T {
  const result = schema.safeParse(value)
  if (!result.success) {
    throw new Error(`unexpected response shape: ${endpoint}`, { cause: result.error })
  }
  return result.data
}
