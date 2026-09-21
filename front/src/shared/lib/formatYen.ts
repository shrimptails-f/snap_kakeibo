const yenFormatter = new Intl.NumberFormat('ja-JP', { style: 'currency', currency: 'JPY' })

// 円の整数を通貨表記にする(例: 2500 → ¥2,500)
export function formatYen(amount: number): string {
  return yenFormatter.format(amount)
}
