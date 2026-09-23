import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { getExpense } from '../api/expenses.api'
import { Button } from '@/shared/ui/Button'
import styles from './ReceiptImage.module.css'

type Props = {
  expenseId: string
  initialUrl?: string
}

type ImageStatus = 'unavailable' | 'loading' | 'ready' | 'error'

export function ReceiptImage({ expenseId, initialUrl }: Props) {
  const [imageUrl, setImageUrl] = useState(initialUrl)
  const [status, setStatus] = useState<ImageStatus>(initialUrl ? 'loading' : 'unavailable')
  const [isRetrying, setIsRetrying] = useState(false)
  const [isMobileOpen, setIsMobileOpen] = useState(false)
  const [isDialogOpen, setIsDialogOpen] = useState(false)
  const [zoom, setZoom] = useState(1)
  const dialogRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!isDialogOpen) return
    const dialog = dialogRef.current
    dialog?.querySelector<HTMLElement>('button')?.focus()
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault()
        setIsDialogOpen(false)
        return
      }
      if (event.key !== 'Tab' || !dialog) return
      const focusable = [...dialog.querySelectorAll<HTMLElement>('button:not([disabled])')]
      if (focusable.length === 0) return
      const first = focusable[0]
      const last = focusable[focusable.length - 1]
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault(); last.focus()
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault(); first.focus()
      }
    }
    document.addEventListener('keydown', handleKeyDown)
    return () => {
      document.removeEventListener('keydown', handleKeyDown)
      document.getElementById('receipt-expand-button')?.focus()
    }
  }, [isDialogOpen])

  async function retryImage() {
    setIsRetrying(true)
    try {
      const fresh = await getExpense(expenseId)
      setImageUrl(fresh.expense.image_url)
      setStatus(fresh.expense.image_url ? 'loading' : 'unavailable')
    } catch {
      setStatus('error')
    } finally {
      setIsRetrying(false)
    }
  }

  function openDialog() {
    setZoom(1)
    setIsDialogOpen(true)
  }

  function closeDialog() {
    setIsDialogOpen(false)
  }

  const image = imageUrl && (
    <img
      className={styles.image}
      hidden={status === 'error'}
      src={imageUrl}
      alt="この支出のレシート画像"
      onLoad={() => setStatus('ready')}
      onError={() => { setStatus('error'); setIsDialogOpen(false) }}
    />
  )

  return (
    <div className={styles.root}>
      <Button
        className={styles.mobileToggle}
        variant="secondary"
        aria-expanded={isMobileOpen}
        aria-controls="receipt-image-content"
        onClick={() => setIsMobileOpen((open) => !open)}
      >
        {isMobileOpen ? 'レシート画像を閉じる' : 'レシート画像を確認'}
      </Button>
      <div id="receipt-image-content" className={isMobileOpen ? styles.contentOpen : styles.content}>
        {status === 'unavailable' ? (
          <div className={styles.message}><p>レシート画像は現在表示できません。</p><small>画像URLを取得できませんでした。</small></div>
        ) : (
          <>
            <div className={styles.viewport}>
              {image}
              {status === 'loading' && <p className={styles.loading} role="status">レシート画像を読み込み中</p>}
              {status === 'error' && <div className={styles.message} role="alert"><p>画像を表示できません。</p><Button variant="secondary" disabled={isRetrying} onClick={retryImage}>{isRetrying ? '再試行中…' : '画像を再読み込み'}</Button></div>}
            </div>
            {status === 'ready' && <Button id="receipt-expand-button" variant="secondary" onClick={openDialog}>拡大して見る</Button>}
          </>
        )}
      </div>

      {isDialogOpen && imageUrl && createPortal((
        <div className={styles.dialogBackdrop} onMouseDown={(event) => { if (event.target === event.currentTarget) closeDialog() }}>
          <div ref={dialogRef} className={styles.dialog} role="dialog" aria-modal="true" aria-labelledby="receipt-dialog-title">
            <div className={styles.dialogToolbar}>
              <h2 id="receipt-dialog-title">レシート画像</h2>
              <div>
                <Button variant="secondary" disabled={zoom <= 0.5} onClick={() => setZoom((value) => Math.max(0.5, value - 0.25))}>縮小</Button>
                <Button variant="secondary" onClick={() => setZoom(1)}>全体を表示</Button>
                <Button variant="secondary" disabled={zoom >= 2.5} onClick={() => setZoom((value) => Math.min(2.5, value + 0.25))}>拡大</Button>
                <Button variant="primary" onClick={closeDialog}>閉じる</Button>
              </div>
            </div>
            <div className={styles.dialogViewport}>
              <img src={imageUrl} alt="この支出のレシート画像（拡大表示）" style={{ width: `${zoom * 100}%` }} onError={() => { setStatus('error'); closeDialog() }} />
            </div>
          </div>
        </div>
      ), document.body)}
    </div>
  )
}
