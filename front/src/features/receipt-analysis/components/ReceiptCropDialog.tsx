import { useEffect, useRef } from 'react'
import type { PointerEvent, KeyboardEvent } from 'react'
import { Button } from '@/shared/ui/Button'
import { useReceiptCrop } from '../hooks/useReceiptCrop'
import { adjustCrop, FULL_CROP } from '../lib/receiptCrop'
import type { CropHandle, CropRect } from '../lib/receiptCrop'
import type { SelectedReceipt } from '../hooks/useReceiptUploadBatch'
import styles from './ReceiptCropDialog.module.css'

const handles: { handle: CropHandle; label: string; x: number; y: number }[] = [
  { handle: 'nw', label: '左上', x: 0, y: 0 }, { handle: 'ne', label: '右上', x: 100, y: 0 },
  { handle: 'sw', label: '左下', x: 0, y: 100 }, { handle: 'se', label: '右下', x: 100, y: 100 },
]

type Props = {
  receipt: SelectedReceipt
  onApply: (file: File, crop?: CropRect) => void
  onClose: () => void
}

export function ReceiptCropDialog({ receipt, onApply, onClose }: Props) {
  const editor = useReceiptCrop(receipt.originalFile, receipt.originalUrl, receipt.crop)
  const dialog = useRef<HTMLDialogElement>(null)
  const stage = useRef<HTMLDivElement>(null)
  const drag = useRef<{ pointerId: number; x: number; y: number; rect: CropRect; handle: CropHandle; width: number; height: number } | null>(null)
  useEffect(() => {
    const opener = document.activeElement
    const element = dialog.current
    element?.showModal()
    return () => {
      element?.close()
      if (opener instanceof HTMLElement && opener.isConnected) opener.focus()
    }
  }, [])

  function startDrag(event: PointerEvent<HTMLButtonElement>, handle: CropHandle) {
    if (editor.isSaving || !event.isPrimary || event.button !== 0) return
    const bounds = stage.current?.getBoundingClientRect()
    if (!bounds) return
    event.preventDefault()
    event.currentTarget.focus()
    event.currentTarget.setPointerCapture(event.pointerId)
    editor.changeRect(editor.rect)
    drag.current = { pointerId: event.pointerId, x: event.clientX, y: event.clientY, rect: editor.rect, handle, width: bounds.width, height: bounds.height }
  }
  function moveDrag(event: PointerEvent<HTMLButtonElement>) {
    const start = drag.current
    if (!start || start.pointerId !== event.pointerId) return
    editor.changeRect(adjustCrop(start.rect, start.handle, (event.clientX - start.x) / start.width, (event.clientY - start.y) / start.height))
  }
  function adjustWithKeyboard(event: KeyboardEvent<HTMLButtonElement>, handle: CropHandle) {
    const directions: Record<string, [number, number]> = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] }
    const direction = directions[event.key]
    if (!direction || editor.isSaving) return
    event.preventDefault()
    const step = event.shiftKey ? 0.05 : 0.005
    editor.changeRect(adjustCrop(editor.rect, handle, direction[0] * step, direction[1] * step))
  }
  const pointerEvents = (handle: CropHandle) => ({
    onPointerDown: (event: PointerEvent<HTMLButtonElement>) => startDrag(event, handle),
    onPointerMove: moveDrag,
    onPointerUp: () => { drag.current = null },
    onPointerCancel: () => { drag.current = null },
    onLostPointerCapture: () => { drag.current = null },
    onKeyDown: (event: KeyboardEvent<HTMLButtonElement>) => adjustWithKeyboard(event, handle),
  })
  async function apply() {
    const result = await editor.exportCrop()
    if (result) onApply(result.file, result.crop)
  }
  const { rect } = editor
  return (
    <dialog ref={dialog} className={styles.dialog} aria-labelledby="crop-title" aria-describedby="crop-help" onCancel={(event) => { event.preventDefault(); onClose() }}>
      <h2 id="crop-title">余白を切り取る</h2>
      <p className={styles.fileName}>{receipt.originalFile.name}</p>
      <p id="crop-help">枠の内側で移動、四隅でサイズを調整できます。キーボードでは枠・四隅に移動して矢印キー（Shiftで大きく調整）を使います。</p>
      <p role="status">{editor.message}</p>
      {editor.size && (
        <div className={styles.stage} ref={stage} style={{ aspectRatio: `${editor.size.width} / ${editor.size.height}`, width: `min(calc(100% - 44px), ${55 * editor.size.width / editor.size.height}vh)` }}>
          <img src={receipt.originalUrl} alt="切り取り前のレシート" draggable={false} />
          <div className={styles.selection} style={{ left: `${rect.x * 100}%`, top: `${rect.y * 100}%`, width: `${rect.width * 100}%`, height: `${rect.height * 100}%` }}>
            <button type="button" className={styles.move} aria-label="切り取り枠を移動" disabled={editor.isSaving} {...pointerEvents('move')} />
            {handles.map(({ handle, label, x, y }) => <button key={handle} type="button" className={styles.handle} style={{ left: `${x}%`, top: `${y}%` }} aria-label={`${label}の切り取り位置を調整`} disabled={editor.isSaving} {...pointerEvents(handle)} />)}
          </div>
        </div>
      )}
      <p className={styles.hint}>適用した画像が保存・解析されます。送信前なら元画像からやり直せます。</p>
      {editor.error && <p role="alert">{editor.error}</p>}
      <div className={styles.actions}>
        <Button variant="primary" onClick={apply} disabled={!editor.size || editor.isSaving}>{editor.isSaving ? '画像を作成中…' : '適用'}</Button>
        <Button variant="secondary" onClick={() => editor.changeRect(FULL_CROP)} disabled={editor.isSaving}>元に戻す</Button>
        <Button variant="secondary" onClick={() => onApply(receipt.originalFile)} disabled={editor.isSaving}>切り取らずに使う</Button>
        <Button variant="secondary" onClick={onClose}>キャンセル</Button>
      </div>
    </dialog>
  )
}
