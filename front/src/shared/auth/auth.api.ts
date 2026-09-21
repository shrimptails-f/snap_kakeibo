import { Client } from '@/shared/api/client'
import { apiBaseUrl } from '@/shared/config/env'
import { clearAuthToken, isAuthSessionResponse, setAuthSession } from './token'

export const authRefreshEndpoint = '/api/auth/refresh'

// refresh は Cookie の refresh token だけで認証するため、Bearer を付けず 401 でも再送しない専用 client を使う。
// shared/api/http.ts の apiClient から呼ばれるので、http.ts を import しない(循環参照を避ける)
const refreshClient = new Client({ baseUrl: apiBaseUrl, defaultCredentials: 'include' })

// access token を取り直す。成功したらメモリへ保存して true、失敗したら token を消して false を返す。
// false は「未ログイン」を意味し、呼び出し側はログイン画面へ誘導する
export async function refreshAuthSession(): Promise<boolean> {
  try {
    const session = await refreshClient.request<unknown>('POST', authRefreshEndpoint)
    if (!isAuthSessionResponse(session)) {
      clearAuthToken()
      return false
    }
    setAuthSession(session)
    return true
  } catch {
    // refresh token が無い・失効している(401)場合も通信失敗も未ログイン扱いにする
    clearAuthToken()
    return false
  }
}
