import { useCallback, useEffect, useState } from 'react'
import type { ChangeEvent, FormEvent } from 'react'

const API_BASE = import.meta.env.VITE_API_BASE_URL ?? ''

type UploadItem = {
  upload_id: string
  billing_id?: string
  status: 'UPLOADING' | 'ANALYZING' | 'SUCCEEDED' | 'NO_DATA' | 'FAILED'
  file_name: string
  year_month: string
  error_code?: string
  error_message?: string
  created_at: string
  updated_at: string
}

type Billing = {
  billing_id: string
  store_name: string
  purchased_at: string
  final_amount: number
}

type Detail = {
  detail_id: string
  name: string
  category: string
  amount: number
  quantity: number
}

type AuthUser = {
  user_id: string
  email: string
}

type AuthResponse = {
  access_token: string
  token_type: 'Bearer'
  expires_in: number
  user?: AuthUser
}

function currentMonth() {
  return new Date().toISOString().slice(0, 7)
}

function yen(value: number) {
  return new Intl.NumberFormat('ja-JP', { style: 'currency', currency: 'JPY' }).format(value)
}

async function readJSON<T>(path: string, init?: RequestInit, accessToken?: string): Promise<T> {
  const headers = new Headers(init?.headers)
  if (accessToken) headers.set('authorization', `Bearer ${accessToken}`)
  const res = await fetch(`${API_BASE}${path}`, { ...init, headers, credentials: 'include' })
  if (!res.ok) {
    throw new Error(`${res.status} ${res.statusText}`)
  }
  return (await res.json()) as T
}

export default function App() {
  const [accessToken, setAccessToken] = useState<string | null>(null)
  const [user, setUser] = useState<AuthUser | null>(null)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [month] = useState(currentMonth)
  const [uploads, setUploads] = useState<UploadItem[]>([])
  const [selectedBillingID, setSelectedBillingID] = useState<string | null>(null)
  const [billing, setBilling] = useState<Billing | null>(null)
  const [details, setDetails] = useState<Detail[]>([])
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  const refreshAccessToken = useCallback(async () => {
    const data = await readJSON<AuthResponse>('/api/auth/refresh', { method: 'POST' })
    setAccessToken(data.access_token)
    return data.access_token
  }, [])

  const authorizedJSON = useCallback(
    async <T,>(path: string, init?: RequestInit): Promise<T> => {
      try {
        return await readJSON<T>(path, init, accessToken ?? undefined)
      } catch (e) {
        if (!(e instanceof Error) || !e.message.startsWith('401')) throw e
        const nextToken = await refreshAccessToken()
        return readJSON<T>(path, init, nextToken)
      }
    },
    [accessToken, refreshAccessToken],
  )

  const refreshUploads = useCallback(async () => {
    if (!accessToken) return
    const data = await authorizedJSON<{ items: UploadItem[] }>(`/api/months/${month}/uploads`)
    setUploads(data.items)
  }, [accessToken, authorizedJSON, month])

  useEffect(() => {
    refreshAccessToken()
      .then((token) => readJSON<{ user: AuthUser }>('/api/auth/check', undefined, token))
      .then((data) => setUser(data.user))
      .catch(() => undefined)
  }, [refreshAccessToken])

  useEffect(() => {
    refreshUploads().catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)))
    const timer = window.setInterval(() => {
      refreshUploads().catch(() => undefined)
    }, 5000)
    return () => window.clearInterval(timer)
  }, [month, refreshUploads])

  useEffect(() => {
    if (!selectedBillingID) {
      setBilling(null)
      setDetails([])
      return
    }
    authorizedJSON<{ billing: Billing; details: Detail[] }>(`/api/billings/${selectedBillingID}`)
      .then((data) => {
        setBilling(data.billing)
        setDetails(data.details)
      })
      .catch((e: unknown) => setError(e instanceof Error ? e.message : String(e)))
  }, [authorizedJSON, selectedBillingID])

  async function login(e: FormEvent<HTMLFormElement>) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      const data = await readJSON<AuthResponse>('/api/auth/login', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ email, password }),
      })
      setAccessToken(data.access_token)
      setUser(data.user ?? { user_id: '', email })
      setPassword('')
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  async function logout() {
    await readJSON('/api/auth/logout', { method: 'POST' }).catch(() => undefined)
    setAccessToken(null)
    setUser(null)
    setUploads([])
    setSelectedBillingID(null)
  }

  async function upload(file: File) {
    setBusy(true)
    setError(null)
    setMessage(null)
    try {
      const data = await authorizedJSON<{ put_url: string; upload_id: string }>('/api/uploads', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ file_name: file.name, content_type: file.type || 'image/jpeg' }),
      })
      const put = await fetch(data.put_url, {
        method: 'PUT',
        headers: { 'content-type': file.type || 'image/jpeg' },
        body: file,
      })
      if (!put.ok) throw new Error(`S3 upload failed: ${put.status} ${put.statusText}`)
      setMessage('アップロードしました。解析が完了すると履歴に反映されます。')
      await refreshUploads()
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  function onFileChange(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    if (file) void upload(file)
    e.target.value = ''
  }

  return (
    <main>
      <header>
        <div>
          <h1>snap_kakeibo</h1>
          <p>{month} のレシート取り込み</p>
        </div>
        <label className="uploadButton">
          <input type="file" accept="image/*,.pdf" onChange={onFileChange} disabled={busy || !accessToken} />
          {busy ? 'アップロード中...' : accessToken ? 'ファイルを選択' : 'ログインしてください'}
        </label>
      </header>

      {!accessToken && (
        <section className="loginPanel">
          <form onSubmit={login}>
            <h2>ログイン</h2>
            <input
              autoComplete="email"
              inputMode="email"
              onChange={(e) => setEmail(e.target.value)}
              placeholder="メールアドレス"
              required
              type="email"
              value={email}
            />
            <input
              autoComplete="current-password"
              onChange={(e) => setPassword(e.target.value)}
              placeholder="パスワード"
              required
              type="password"
              value={password}
            />
            <button disabled={busy} type="submit">
              {busy ? 'ログイン中...' : 'ログイン'}
            </button>
          </form>
        </section>
      )}

      {accessToken && (
        <div className="sessionBar">
          <span>{user?.email}</span>
          <button type="button" onClick={logout}>
            ログアウト
          </button>
        </div>
      )}

      {message && <p className="notice">{message}</p>}
      {error && <p className="error">Error: {error}</p>}

      {accessToken && (
        <section className="layout">
        <div className="panel">
          <h2>アップロード履歴</h2>
          <div className="list">
            {uploads.length === 0 && <p className="muted">まだ履歴がありません。</p>}
            {uploads.map((item) => (
              <button
                className="row"
                key={item.upload_id}
                type="button"
                disabled={!item.billing_id}
                onClick={() => item.billing_id && setSelectedBillingID(item.billing_id)}
              >
                <span>
                  <strong>{item.file_name || item.upload_id}</strong>
                  <small>{new Date(item.created_at).toLocaleString('ja-JP')}</small>
                  {item.error_message && <small>{item.error_message}</small>}
                </span>
                <span className={`status ${item.status.toLowerCase()}`}>{item.status}</span>
              </button>
            ))}
          </div>
        </div>

        <div className="panel">
          <h2>詳細</h2>
          {!billing && <p className="muted">解析完了した履歴を選択してください。</p>}
          {billing && (
            <>
              <div className="summary">
                <span>{billing.store_name}</span>
                <strong>{yen(billing.final_amount)}</strong>
                <small>{billing.purchased_at}</small>
              </div>
              <table>
                <thead>
                  <tr>
                    <th>品目</th>
                    <th>カテゴリ</th>
                    <th>金額</th>
                  </tr>
                </thead>
                <tbody>
                  {details.map((detail) => (
                    <tr key={detail.detail_id}>
                      <td>{detail.name}</td>
                      <td>{detail.category}</td>
                      <td>{yen(detail.amount)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </>
          )}
        </div>
        </section>
      )}
    </main>
  )
}
