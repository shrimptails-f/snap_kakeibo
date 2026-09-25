import type { CropRect } from './receiptCrop'

type Candidate = { rect: CropRect; score: number; preservesHeight: boolean }

// 見切れた候補は内部の文字のエッジではなく、左右の外周が背景から分かれるかを確かめる。
function hasClearSides(indices: Int32Array, light: Float32Array, width: number, top: number, bottom: number): boolean {
  const lefts = new Int32Array(bottom - top + 1).fill(width)
  const rights = new Int32Array(bottom - top + 1).fill(-1)
  for (const index of indices) {
    const row = Math.floor(index / width) - top
    const x = index % width
    lefts[row] = Math.min(lefts[row], x)
    rights[row] = Math.max(rights[row], x)
  }
  let clearRows = 0
  for (let row = 0; row < lefts.length; row++) {
    const left = lefts[row], right = rights[row]
    if (left < 4 || right > width - 5 || right - left < 8) continue
    const offset = (row + top) * width
    let leftContrast = 0, rightContrast = 0
    for (let distance = 1; distance <= 4; distance++) {
      leftContrast += light[offset + left + distance] - light[offset + left - distance]
      rightContrast += light[offset + right - distance] - light[offset + right + distance]
    }
    if (leftContrast / 4 >= 22 && rightContrast / 4 >= 22) clearRows++
  }
  return clearRows / lefts.length >= 0.7
}

function overlap(a: CropRect, b: CropRect): number {
  const intersection = Math.max(0, Math.min(a.x + a.width, b.x + b.width) - Math.max(a.x, b.x)) * Math.max(0, Math.min(a.y + a.height, b.y + b.height) - Math.max(a.y, b.y))
  return intersection / Math.min(a.width * a.height, b.width * b.height)
}

// 最大480pxの検出専用画像。明度・色の連続領域を輪郭のコントラスト、充填率、面積で評価する。
// 縦横比は条件にしない。下端の見切れは左右の境界が明瞭な場合だけ高さを維持して提案する。
export function detectReceiptBounds(data: Uint8ClampedArray, width: number, height: number): CropRect | null {
  const length = width * height
  if (length === 0 || data.length !== length * 4) return null
  const light = new Float32Array(length)
  const chroma = new Float32Array(length)
  for (let i = 0; i < length; i++) {
    const [r, g, b] = data.subarray(i * 4, i * 4 + 3)
    light[i] = (r + g + b) / 3
    chroma[i] = Math.max(r, g, b) - Math.min(r, g, b)
  }
  const candidates: Candidate[] = []
  const queue = new Int32Array(length)
  for (const threshold of [130, 170, 210]) {
    const seen = new Uint8Array(length)
    const isPaper = (i: number) => light[i] >= threshold && chroma[i] < 65 && data[i * 4 + 3] > 200
    for (let start = 0; start < length; start++) {
      if (seen[start] || !isPaper(start)) continue
      let head = 0, tail = 1
      queue[0] = start
      seen[start] = 1
      let left = width, top = height, right = 0, bottom = 0, edge = 0, perimeter = 0
      while (head < tail) {
        const index = queue[head++]
        const x = index % width, y = Math.floor(index / width)
        left = Math.min(left, x); right = Math.max(right, x)
        top = Math.min(top, y); bottom = Math.max(bottom, y)
        for (const next of [x > 0 ? index - 1 : -1, x < width - 1 ? index + 1 : -1, y > 0 ? index - width : -1, y < height - 1 ? index + width : -1]) {
          if (next < 0) continue
          if (!isPaper(next)) { perimeter++; edge += Math.abs(light[index] - light[next]); continue }
          if (!seen[next]) { seen[next] = 1; queue[tail++] = next }
        }
      }
      const boxArea = (right - left + 1) * (bottom - top + 1)
      const area = boxArea / length, fill = tail / boxArea
      if (area < 0.06 || area > 0.9 || fill < 0.65 || left < 2 || top < 2 || right > width - 3) continue
      const preservesHeight = bottom > height - 3
      if (preservesHeight && !hasClearSides(queue.subarray(0, tail), light, width, top, bottom)) continue
      const contrast = edge / Math.max(perimeter, 1)
      if (contrast < 22) continue
      const rect = { x: left / width, y: top / height, width: (right - left + 1) / width, height: (bottom - top + 1) / height }
      const candidate = { rect, preservesHeight, score: fill * Math.min(contrast / 70, 1) * Math.sqrt(area) }
      const duplicate = candidates.find((other) => overlap(other.rect, rect) > 0.9)
      if (!duplicate) candidates.push(candidate)
      // 閾値によって影のない部分だけが候補になっても、外側の紙を優先する。
      else if (rect.width * rect.height > duplicate.rect.width * duplicate.rect.height) Object.assign(duplicate, candidate)
    }
  }
  candidates.sort((a, b) => b.score - a.score)
  const best = candidates[0]
  if (!best || best.score < 0.12 || (candidates[1] && candidates[1].score > best.score * 0.65)) return null
  // 店名・日付・合計を落としにくいよう各辺に画像寸法の3%の余白を加える。
  const x = Math.max(0, best.rect.x - 0.03)
  const y = best.preservesHeight ? 0 : Math.max(0, best.rect.y - 0.03)
  return { x, y, width: Math.min(1, best.rect.x + best.rect.width + 0.03) - x, height: best.preservesHeight ? 1 : Math.min(1, best.rect.y + best.rect.height + 0.03) - y }
}
