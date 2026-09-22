import { Navigate, Outlet, useLocation } from 'react-router'
import { useAuthSession } from '@/features/auth'
import { SessionCheckingNotice } from './SessionCheckingNotice'

const HOME_PATH = '/'

// AuthGuard が /login へ送るときに残した戻り先。外部から渡された state は信用せず、アプリ内パスだけ受け付ける
function readReturnPath(state: unknown): string {
  if (typeof state !== 'object' || state === null) return HOME_PATH
  const from = (state as Record<string, unknown>).from
  return typeof from === 'string' && from.startsWith('/') && !from.startsWith('//') ? from : HOME_PATH
}

// 未ログイン専用のルート(/login)を包む。ログイン済みなら元の URL か / へ戻す
export function GuestGuard() {
  const location = useLocation()
  const { isChecking, isAuthorized } = useAuthSession()

  if (isChecking) {
    return <SessionCheckingNotice />
  }
  if (isAuthorized) {
    return <Navigate to={readReturnPath(location.state)} replace />
  }
  return <Outlet />
}
