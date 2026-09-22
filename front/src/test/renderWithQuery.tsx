import { Suspense } from 'react'
import type { ReactNode } from 'react'
import { QueryClientProvider } from '@tanstack/react-query'
import { render } from '@testing-library/react'
import { createQueryClient } from '@/app/providers/queryClient'
import { SpinnerBlock } from '@/shared/ui/Spinner'

// useSuspenseQuery を使う画面・部品を、テストごとに新しい cache と Suspense 境界で描画する
export function renderWithQuery(ui: ReactNode) {
  const queryClient = createQueryClient()
  const result = render(
    <QueryClientProvider client={queryClient}>
      <Suspense fallback={<SpinnerBlock label="画面を読み込んでいます" />}>{ui}</Suspense>
    </QueryClientProvider>,
  )
  return { ...result, queryClient }
}
