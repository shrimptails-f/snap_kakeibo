export type CropRect = { x: number; y: number; width: number; height: number }
export type CropHandle = 'move' | 'nw' | 'ne' | 'sw' | 'se'
export const FULL_CROP: CropRect = { x: 0, y: 0, width: 1, height: 1 }
const MIN_SIZE = 0.02
const clamp = (value: number, min: number, max: number) => Math.max(min, Math.min(max, value))

// 座標は向き補正済み画像に対する割合。表示サイズから独立して保持する。
export function adjustCrop(rect: CropRect, handle: CropHandle, dx: number, dy: number): CropRect {
  if (handle === 'move') return { ...rect, x: clamp(rect.x + dx, 0, 1 - rect.width), y: clamp(rect.y + dy, 0, 1 - rect.height) }
  let { x, y } = rect
  let right = x + rect.width
  let bottom = y + rect.height
  if (handle.includes('w')) x = clamp(x + dx, 0, right - MIN_SIZE)
  if (handle.includes('e')) right = clamp(right + dx, x + MIN_SIZE, 1)
  if (handle.includes('n')) y = clamp(y + dy, 0, bottom - MIN_SIZE)
  if (handle.includes('s')) bottom = clamp(bottom + dy, y + MIN_SIZE, 1)
  return { x, y, width: right - x, height: bottom - y }
}

export function cropPixels(rect: CropRect, width: number, height: number): CropRect {
  const x = clamp(Math.floor(rect.x * width), 0, width - 1)
  const y = clamp(Math.floor(rect.y * height), 0, height - 1)
  return { x, y, width: clamp(Math.ceil((rect.x + rect.width) * width) - x, 1, width - x), height: clamp(Math.ceil((rect.y + rect.height) * height) - y, 1, height - y) }
}

export function validateReceiptFile(file: File): string | undefined {
  if (!['image/jpeg', 'image/png'].includes(file.type)) return 'JPEG または PNG の画像を選んでください。'
  // S3の30 MiB制限はmultipart全体なので、フォーム分の余裕を残す。
  if (file.size > (29 << 20)) return '画像は 29 MiB 以下にしてください。'
  if (file.size === 0) return '空の画像は送信できません。'
}
