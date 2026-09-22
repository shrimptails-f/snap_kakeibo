import { describe, expect, it } from 'vitest'
import { z } from 'zod'
import { parseResponse } from './parseResponse'

const schema = z.object({ items: z.array(z.string()) })

describe('parseResponse', () => {
  it('形が合えば検証済みの値を返し、未知のフィールドは落とす', () => {
    expect(parseResponse(schema, { items: ['a'], extra: 1 }, 'GET /api/x')).toEqual({ items: ['a'] })
  })

  it('形が違えば endpoint 付きの Error を投げ、zod の詳細は cause に残す', () => {
    expect(() => parseResponse(schema, { items: 'a' }, 'GET /api/x')).toThrowError(
      expect.objectContaining({ message: 'unexpected response shape: GET /api/x', cause: expect.any(z.ZodError) }),
    )
  })
})
