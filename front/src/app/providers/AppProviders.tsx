import { useState } from 'react'
import type { ReactNode } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { AuthSessionProvider } from '@/features/auth'
import { createQueryClient } from './queryClient'

type Props = {
  children: ReactNode
}

// アプリ全体の Provider を合成する。Router より外側に置き、ガードから認証セッションを参照できるようにする
export function AppProviders({ children }: Props) {
  const [queryClient] = useState(createQueryClient)

  return (
    <QueryClientProvider client={queryClient}>
      <AuthSessionProvider>{children}</AuthSessionProvider>
    </QueryClientProvider>
  )
}
