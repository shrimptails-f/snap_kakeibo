// 環境変数はここで一度だけ解決し、他のモジュールは import.meta.env を直接参照しない

// API の origin。未設定なら同一 origin の /api/* を相対パスで呼ぶ
export const apiBaseUrl: string = import.meta.env.VITE_API_BASE_URL ?? ''
