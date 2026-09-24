import { describe, expect, it } from 'vitest'
import { adjustCrop, cropPixels, FULL_CROP, validateReceiptFile } from './receiptCrop'
import { detectReceiptBounds } from './detectReceiptBounds'

describe('切り取り座標', () => {
  it('移動でサイズを保ち画像外へ出さない', () => {
    expect(adjustCrop({ x: 0.1, y: 0.2, width: 0.5, height: 0.4 }, 'move', 2, -2)).toEqual({ x: 0.5, y: 0, width: 0.5, height: 0.4 })
  })
  it('四隅の反転と画像外への拡張を防ぐ', () => {
    const small = adjustCrop(FULL_CROP, 'nw', 2, 2)
    expect(small.width).toBeCloseTo(0.02)
    expect(small.height).toBeCloseTo(0.02)
    expect(adjustCrop(FULL_CROP, 'se', 2, 2)).toEqual(FULL_CROP)
    expect(adjustCrop(FULL_CROP, 'ne', -0.2, 0.3)).toEqual({ x: 0, y: 0.3, width: 0.8, height: 0.7 })
    expect(adjustCrop(FULL_CROP, 'sw', 0.2, -0.3)).toEqual({ x: 0.2, y: 0, width: 0.8, height: 0.7 })
  })
  it('縦長・横長・1px画像で端の画素を欠落させない', () => {
    for (const [width, height] of [[4032, 3024], [3024, 4032], [1, 1]]) expect(cropPixels(FULL_CROP, width, height)).toEqual({ x: 0, y: 0, width, height })
    expect(cropPixels({ x: 0.101, y: 0.201, width: 0.3, height: 0.4 }, 100, 200)).toEqual({ x: 10, y: 40, width: 31, height: 81 })
  })
  it('確定ファイルの形式・サイズを再検証する', () => {
    expect(validateReceiptFile(new File(['x'], 'ok.png', { type: 'image/png' }))).toBeUndefined()
    expect(validateReceiptFile(new File(['x'], 'bad.webp', { type: 'image/webp' }))).toMatch(/JPEG/)
    expect(validateReceiptFile(new File([], 'empty.png', { type: 'image/png' }))).toMatch(/空/)
    const large = new File(['x'], 'large.jpg', { type: 'image/jpeg' })
    Object.defineProperty(large, 'size', { value: (29 << 20) + 1 })
    expect(validateReceiptFile(large)).toMatch(/29 MiB/)
  })
})

function fixture(background: number, papers: [number, number, number, number][], shadow = false, tilted = false) {
  const width = 120, height = 160
  const data = new Uint8ClampedArray(width * height * 4)
  for (let y = 0; y < height; y++) for (let x = 0; x < width; x++) {
    const shift = tilted ? Math.floor(y / 10) : 0
    const paper = papers.some(([left, top, right, bottom]) => x >= left + shift && x < right + shift && y >= top && y < bottom)
    const value = paper ? (shadow && x < 50 ? 180 : 240) : background
    data.set([value, value, value, 255], (y * width + x) * 4)
  }
  return detectReceiptBounds(data, width, height)
}

describe('範囲の自動提案', () => {
  it('長い紙と横長の紙を余白付きで提案する', () => {
    for (const paper of [[40, 10, 75, 150], [10, 40, 110, 100]] as [number, number, number, number][]) {
      const rect = fixture(50, [paper])!
      expect(rect).not.toBeNull()
      expect(rect.x).toBeLessThan(paper[0] / 120)
      expect(rect.y).toBeLessThan(paper[1] / 160)
      expect(rect.x + rect.width).toBeGreaterThan(paper[2] / 120)
      expect(rect.y + rect.height).toBeGreaterThan(paper[3] / 160)
    }
  })
  it('影と軽い傾きでも紙の外接範囲を含める', () => {
    const rect = fixture(40, [[25, 15, 80, 145]], true, true)!
    expect(rect).not.toBeNull()
    expect(rect.x).toBeLessThan(26 / 120)
    expect(rect.x + rect.width).toBeGreaterThan(94 / 120)
  })
  it('白背景・複数候補・境界欠け・候補なしでは断定しない', () => {
    expect(fixture(240, [[30, 10, 80, 150]])).toBeNull()
    expect(fixture(40, [[10, 10, 45, 150], [75, 10, 110, 150]])).toBeNull()
    expect(fixture(40, [[30, 0, 80, 150]])).toBeNull()
    expect(fixture(40, [])).toBeNull()
  })
})
