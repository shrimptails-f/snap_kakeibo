import { describe, expect, it } from 'vitest'
import { ApiError } from './client'
import { toFriendlyMessage } from './errors'

describe('toFriendlyMessage', () => {
  it.each([
    [401, 'ログインの有効期限が切れました。再度ログインしてください。'],
    [400, '入力内容に誤りがあります。内容を確認して再度お試しください。'],
    [404, '対象のデータが見つかりません。'],
    [429, 'リクエストが集中しています。しばらく待ってから再度お試しください。'],
    [500, 'サーバーでエラーが発生しました。時間をおいて再度お試しください。'],
    [503, 'サーバーでエラーが発生しました。時間をおいて再度お試しください。'],
  ])('status %i は利用者向けの文言にする', (status, expected) => {
    expect(toFriendlyMessage(new ApiError({ status, apiMessage: 'unauthorized' }))).toBe(expected)
  })

  it('バックエンドの英文メッセージや status をそのまま出さない', () => {
    const message = toFriendlyMessage(new ApiError({ status: 401, apiMessage: 'invalid email or password' }))

    expect(message).not.toContain('invalid email or password')
    expect(message).not.toContain('401')
  })

  it('ApiError 以外(通信断など)は通信失敗の文言にする', () => {
    expect(toFriendlyMessage(new TypeError('Failed to fetch'))).toBe('通信に失敗しました。接続を確認して再度お試しください。')
    expect(toFriendlyMessage(undefined)).toBe('通信に失敗しました。接続を確認して再度お試しください。')
  })
})
