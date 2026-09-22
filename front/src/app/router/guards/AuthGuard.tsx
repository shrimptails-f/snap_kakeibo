import { Navigate, Outlet, useLocation } from 'react-router'
import { useAuthSession } from '@/features/auth'
import { SessionCheckingNotice } from './SessionCheckingNotice'

export const LOGIN_PATH = '/login'

// ログインが必要なルートを包む。未ログインなら /login へ送り、戻り先を state.from に残す
export function AuthGuard() {
  const location = useLocation()
  const { isChecking, isUnauthorized } = useAuthSession()

  if (isChecking) {
    return <SessionCheckingNotice />
  }
  if (isUnauthorized) {
    return <Navigate to={LOGIN_PATH} replace state={{ from: `${location.pathname}${location.search}` }} />
  }
  return <Outlet />
}
