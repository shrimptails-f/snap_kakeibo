/// <reference types="vite/client" />

// import.meta.env の VITE_* を型付けする。読み取りは shared/config に集約する
interface ImportMetaEnv {
  readonly VITE_API_BASE_URL?: string
}

interface ImportMeta {
  readonly env: ImportMetaEnv
}
