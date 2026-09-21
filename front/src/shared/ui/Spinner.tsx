import './Spinner.css'

type Props = {
  // 直径(px)
  size?: number
  // 読み上げ用。何を待っているかが分かる文言にする
  label?: string
}

// 短い待ちの進行表示。長い待ちや領域の形が分かっている場合は skeleton を検討する
export function Spinner({ size = 24, label = '読み込み中' }: Props) {
  return (
    <svg
      role="status"
      aria-label={label}
      className="spinner"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      <circle className="spinner__track" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
      <path fill="currentColor" d="M4 12a8 8 0 0 1 8-8v4a4 4 0 0 0-4 4H4z" />
    </svg>
  )
}

type BlockProps = Props

// 画面やパネルの本文領域を占める loading 表示(Suspense の fallback 向け)
export function SpinnerBlock({ size = 32, label }: BlockProps) {
  return (
    <div className="spinner-block">
      <Spinner size={size} label={label} />
    </div>
  )
}
