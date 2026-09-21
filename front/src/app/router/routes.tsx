import { Navigate, type RouteObject } from 'react-router'
import { LoginPage } from '@/features/auth'
import { ReceiptIntakePage } from '@/features/receipt-analysis'
import { AppLayout } from '../layouts/AppLayout'
import { AuthGuard, LOGIN_PATH } from './guards/AuthGuard'
import { GuestGuard } from './guards/GuestGuard'

// ルート定義。/login 以外はすべて AuthGuard の配下に置く。
// /months/:yearMonth、/analysis-requests、/expenses/:expenseId は画面の実装時に追加する
export const routes: RouteObject[] = [
  {
    element: <GuestGuard />,
    children: [{ path: LOGIN_PATH, element: <LoginPage /> }],
  },
  {
    element: <AuthGuard />,
    children: [
      {
        element: <AppLayout />,
        children: [{ index: true, element: <ReceiptIntakePage /> }],
      },
    ],
  },
  // 未定義の URL は Router の既定エラー画面を出さず、/ へ戻す(未ログインなら AuthGuard が /login へ送る)
  { path: '*', element: <Navigate to="/" replace /> },
]
