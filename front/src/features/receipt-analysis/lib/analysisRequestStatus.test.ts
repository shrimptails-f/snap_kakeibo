import { describe, expect, it } from 'vitest'
import type { AnalysisRequestItem } from '../types/analysis-request.types'
import { analysisRequestStatus } from './analysisRequestStatus'

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

describe('analysisRequestStatus', () => {
  it('UPLOADING は upload_expires_at を過ぎたら期限切れ(warning)にする', () => {
    expect(analysisRequestStatus(item({}), now)).toEqual({ label: 'アップロード待ち', tone: 'neutral' })
    expect(analysisRequestStatus(item({ upload_expires_at: '2026-09-18T00:09:59Z' }), now)).toEqual({
      label: '期限切れ',
      tone: 'warning',
    })
  })

  it.each([
    ['ANALYZING', '解析中', 'info'],
    ['SUCCEEDED', '登録完了', 'primary'],
    ['NO_DATA', '登録対象なし', 'neutral'],
    ['FAILED', '解析失敗', 'danger'],
  ] as const)('%s は %s(%s)', (status, label, tone) => {
    expect(analysisRequestStatus(item({ status }), now)).toEqual({ label, tone })
  })

  it('ANALYZING は更新から30分を過ぎたら停滞にする', () => {
    expect(analysisRequestStatus(item({ status: 'ANALYZING', updated_at: '2026-09-17T23:39:59Z' }), now)).toEqual({
      label: '停滞',
      tone: 'warning',
      isStalled: true,
    })
  })
})
