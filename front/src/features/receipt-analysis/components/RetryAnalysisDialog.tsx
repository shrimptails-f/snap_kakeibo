import { useEffect, useRef } from 'react'
import { Button } from '@/shared/ui/Button'
import styles from './RetryAnalysisDialog.module.css'

type Props = {
  fileName: string
  isStalled: boolean
  isPending: boolean
  onCancel: () => void
  onConfirm: () => void
}

export function RetryAnalysisDialog({ fileName, isStalled, isPending, onCancel, onConfirm }: Props) {
  const cancelRef = useRef<HTMLButtonElement>(null)
  const confirmRef = useRef<HTMLButtonElement>(null)
  useEffect(() => {
    cancelRef.current?.focus()
    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape' && !isPending) onCancel()
      if (event.key === 'Tab') {
        event.preventDefault()
        const next = event.shiftKey || document.activeElement === confirmRef.current ? cancelRef.current : confirmRef.current
        next?.focus()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [isPending, onCancel])

  return (
    <div className={styles.backdrop} role="presentation">
      <div className={styles.dialog} role="alertdialog" aria-modal="true" aria-labelledby="retry-title" aria-describedby="retry-description">
        <h2 id="retry-title">この画像を再解析しますか？</h2>
        <p className={styles.fileName}>{fileName}</p>
        <p id="retry-description">保存済みの画像でもう一度読み取りを行います。</p>
        {isStalled && <p>前の解析が進行中の可能性があります。新しい試行を開始します。</p>}
        <div className={styles.actions}>
          <Button ref={cancelRef} variant="secondary" onClick={onCancel} disabled={isPending}>キャンセル</Button>
          <Button ref={confirmRef} variant="primary" onClick={onConfirm} disabled={isPending}>{isPending ? '再解析を開始中…' : '再解析する'}</Button>
        </div>
      </div>
    </div>
  )
}
