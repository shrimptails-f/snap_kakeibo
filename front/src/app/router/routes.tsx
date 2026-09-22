import { Navigate, type RouteObject } from 'react-router'
import { LoginPage } from '@/features/auth'
import { DashboardPage } from '@/features/dashboard'
import { ExpenseDetailPage, MonthlyExpensesPage } from '@/features/expenses'
import { AnalysisRequestsPage, UploadPage } from '@/features/receipt-analysis'
import { AppLayout } from '../layouts/AppLayout'
import { GuestLayout } from '../layouts/GuestLayout'
import { RouteErrorPage } from './errors/RouteErrorPage'
import { AuthGuard, LOGIN_PATH } from './guards/AuthGuard'
import { GuestGuard } from './guards/GuestGuard'

// ルート定義。/login 以外はすべて AuthGuard の配下に置く。
// レイアウトをガードの外側にし、セッション確認中もヘッダー・フッターは表示したまま本文だけを差し替える。
// errorElement は画面のルートに置く(ガードより内側)。ガードが残るので、セッション切れなら /login へ送れる。
export const routes: RouteObject[] = [
  {
    element: <GuestLayout />,
    children: [{ element: <GuestGuard />, children: [{ path: LOGIN_PATH, element: <LoginPage /> }] }],
  },
  {
    element: <AppLayout />,
    children: [
      {
        element: <AuthGuard />,
        children: [
          { index: true, element: <DashboardPage />, errorElement: <RouteErrorPage /> },
          { path: '/upload', element: <UploadPage />, errorElement: <RouteErrorPage /> },
          { path: '/analysis-requests', element: <AnalysisRequestsPage />, errorElement: <RouteErrorPage /> },
          { path: '/months/:yearMonth', element: <MonthlyExpensesPage />, errorElement: <RouteErrorPage /> },
          { path: '/expenses/:expenseId', element: <ExpenseDetailPage />, errorElement: <RouteErrorPage /> },
        ],
      },
    ],
  },
  // 未定義の URL は Router の既定エラー画面を出さず、/ へ戻す(未ログインなら AuthGuard が /login へ送る)
  { path: '*', element: <Navigate to="/" replace /> },
]
