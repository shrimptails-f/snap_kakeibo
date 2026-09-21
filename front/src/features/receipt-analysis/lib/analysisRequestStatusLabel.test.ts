import { describe, expect, it } from 'vitest'
import type { AnalysisRequestItem } from '../types/analysis-request.types'
import { analysisRequestStatusLabel } from './analysisRequestStatusLabel'

const now = new Date('2026-09-18T00:10:00Z').getTime()

function item(overrides: Partial<AnalysisRequestItem>): AnalysisRequestItem {
  return {
    analysis_request_id: 'req1',
    status: 'UPLOADING',
    attempt: 1,
    file_name: 'receipt.jpg',
    year_month: '2026-09',
    upload_expires_at: '2026-09-18T00:15:00Z',
    created_at: '2026-09-18T00:00:00Z',
    updated_at: '2026-09-18T00:00:00Z',
    ...overrides,
  }
}

describe('analysisRequestStatusLabel', () => {
  it('UPLOADING は upload_expires_at を過ぎたら期限切れにする', () => {
    expect(analysisRequestStatusLabel(item({}), now)).toBe('アップロード待ち')
    expect(analysisRequestStatusLabel(item({ upload_expires_at: '2026-09-18T00:09:59Z' }), now)).toBe('期限切れ')
  })

  it.each([
    ['ANALYZING', '解析中'],
    ['SUCCEEDED', '登録完了'],
    ['NO_DATA', '登録対象なし'],
    ['FAILED', '解析失敗'],
  ] as const)('%s は %s', (status, expected) => {
    expect(analysisRequestStatusLabel(item({ status }), now)).toBe(expected)
  })
})
