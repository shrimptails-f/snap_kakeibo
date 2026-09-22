import { useEffect, useRef } from 'react'
import { Button } from '@/shared/ui/Button'
import styles from './ReceiptPreviewDialog.module.css'

type Props = {
  fileName: string
  previewUrl: string
  onClose: () => void
}

export function ReceiptPreviewDialog({ fileName, previewUrl, onClose }: Props) {
  const closeButtonRef = useRef<HTMLButtonElement>(null)

  useEffect(() => {
    closeButtonRef.current?.focus()
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') onClose()
      if (event.key === 'Tab') {
        event.preventDefault()
        closeButtonRef.current?.focus()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [onClose])

  return (
    <div className={styles.backdrop} role="presentation" onMouseDown={(event) => event.target === event.currentTarget && onClose()}>
      <div className={styles.dialog} role="dialog" aria-modal="true" aria-labelledby="receipt-preview-title">
        <div className={styles.header}>
          <h2 id="receipt-preview-title">{fileName}</h2>
          <Button ref={closeButtonRef} variant="secondary" onClick={onClose}>閉じる</Button>
        </div>
        <img src={previewUrl} alt={`${fileName} のプレビュー`} />
      </div>
    </div>
  )
}
