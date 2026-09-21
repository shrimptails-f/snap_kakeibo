import { QueryClient } from '@tanstack/react-query'

// サーバー状態の cache。401 は shared/api の client が refresh と再送を担うので、Query 側では再試行しない。
// テストごとに cache を分けられるよう、関数で生成する
export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        staleTime: 30 * 1000,
      },
      mutations: {
        retry: false,
      },
    },
  })
}
