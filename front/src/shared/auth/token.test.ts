import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  clearAuthToken,
  getAuthorizationHeaderValue,
  hasAuthToken,
  isAuthSessionResponse,
  setAuthSession,
} from './token'

describe('token', () => {
  afterEach(() => {
    clearAuthToken()
    vi.restoreAllMocks()
  })

  it('access token をメモリにだけ保持し、Web Storage には書かない', () => {
    const getItemSpy = vi.spyOn(Storage.prototype, 'getItem')
    const setItemSpy = vi.spyOn(Storage.prototype, 'setItem')

    setAuthSession({ access_token: 'access-token', token_type: 'Bearer', expires_in: 900 })

    expect(hasAuthToken()).toBe(true)
    expect(getAuthorizationHeaderValue()).toBe('Bearer access-token')
    expect(getItemSpy).not.toHaveBeenCalled()
    expect(setItemSpy).not.toHaveBeenCalled()
  })

  it('token_type が空なら Bearer を使う', () => {
    setAuthSession({ access_token: 'access-token', token_type: '', expires_in: 900 })

    expect(getAuthorizationHeaderValue()).toBe('Bearer access-token')
  })

  it('clearAuthToken で未ログイン状態に戻る', () => {
    setAuthSession({ access_token: 'access-token', token_type: 'Bearer', expires_in: 900 })

    clearAuthToken()

    expect(hasAuthToken()).toBe(false)
    expect(getAuthorizationHeaderValue()).toBeUndefined()
  })
})

describe('isAuthSessionResponse', () => {
  it('login / refresh の応答形式を満たすときだけ true', () => {
    expect(isAuthSessionResponse({ access_token: 'a', token_type: 'Bearer', expires_in: 900 })).toBe(true)
    expect(isAuthSessionResponse({ access_token: 'a', token_type: 'Bearer', expires_in: 900, user: {} })).toBe(true)
  })

  it.each([
    ['access_token が空', { access_token: ' ', token_type: 'Bearer', expires_in: 900 }],
    ['access_token がない', { token_type: 'Bearer', expires_in: 900 }],
    ['expires_in が数値でない', { access_token: 'a', token_type: 'Bearer', expires_in: '900' }],
    ['object でない', 'access-token'],
    ['null', null],
  ])('%s なら false', (_name, value) => {
    expect(isAuthSessionResponse(value)).toBe(false)
  })
})
