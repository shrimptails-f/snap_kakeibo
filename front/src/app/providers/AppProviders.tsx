import type { ReactNode } from 'react'
import { AuthSessionProvider } from '@/features/auth'

type Props = {
  children: ReactNode
}

// アプリ全体の Provider を合成する。Router より外側に置き、ガードから認証セッションを参照できるようにする
export function AppProviders({ children }: Props) {
  return <AuthSessionProvider>{children}</AuthSessionProvider>
}
