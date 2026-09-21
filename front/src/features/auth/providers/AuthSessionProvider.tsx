import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { onUnauthorized } from '@/shared/api/http'
import { login as requestLogin, logout as requestLogout, restoreAuthSession } from '../api/auth.api'
import { AuthSessionContext } from '../hooks/useAuthSession'
import type { AuthSession, AuthSessionStatus } from '../hooks/useAuthSession'
import type { AuthUser, LoginRequest } from '../types/auth.types'

type Props = {
  children: ReactNode
}

type SessionState = {
  status: AuthSessionStatus
  user: AuthUser | null
}

// アプリ全体で 1 つの認証セッションを持つ。起動時に Cookie の refresh token からセッションを復元する
export function AuthSessionProvider({ children }: Props) {
  const [state, setState] = useState<SessionState>({ status: 'checking', user: null })

  useEffect(() => {
    const controller = new AbortController()
    restoreAuthSession(controller.signal)
      .then((response) => {
        if (controller.signal.aborted) return
        setState(response ? { status: 'authorized', user: response.user } : { status: 'unauthorized', user: null })
      })
      .catch(() => {
        // 通信失敗も未ログインとして扱い、ログイン画面へ誘導する
        if (controller.signal.aborted) return
        setState({ status: 'unauthorized', user: null })
      })
    return () => controller.abort()
  }, [])

  // 利用中にセッションが切れた(refresh を経ても 401)ときは、画面ごとに処理せずここで未ログインへ落とす
  useEffect(() => {
    return onUnauthorized(() => setState({ status: 'unauthorized', user: null }))
  }, [])

  async function login(request: LoginRequest): Promise<void> {
    const response = await requestLogin(request)
    setState({ status: 'authorized', user: response.user })
  }

  async function logout(): Promise<void> {
    try {
      await requestLogout()
    } finally {
      setState({ status: 'unauthorized', user: null })
    }
  }

  const session: AuthSession = {
    status: state.status,
    user: state.user,
    isChecking: state.status === 'checking',
    isAuthorized: state.status === 'authorized',
    isUnauthorized: state.status === 'unauthorized',
    login,
    logout,
  }

  return <AuthSessionContext.Provider value={session}>{children}</AuthSessionContext.Provider>
}
