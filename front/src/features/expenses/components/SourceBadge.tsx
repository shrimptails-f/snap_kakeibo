import { useEffect, useId, useRef, useState } from 'react'
import type { ExpenseSource } from '../types/expense.types'
import styles from './SourceBadge.module.css'

type Props = { source: ExpenseSource | undefined; isEdited: boolean; label?: string }

export function SourceBadge({ source, isEdited, label }: Props) {
  const [isOpen, setIsOpen] = useState(false)
  const rootRef = useRef<HTMLSpanElement>(null)
  const isPointerFocus = useRef(false)
  const tooltipId = useId()
  const text = isEdited || source === 'USER' ? '編集済み' : source === 'AI' ? 'AI解析' : '由来不明'

  useEffect(() => {
    if (!isOpen) return
    const closeOutside = (event: PointerEvent) => { if (!rootRef.current?.contains(event.target as Node)) setIsOpen(false) }
    const closeEscape = (event: KeyboardEvent) => { if (event.key === 'Escape') setIsOpen(false) }
    window.addEventListener('pointerdown', closeOutside)
    window.addEventListener('keydown', closeEscape)
    window.addEventListener('scroll', () => setIsOpen(false), { once: true })
    return () => { window.removeEventListener('pointerdown', closeOutside); window.removeEventListener('keydown', closeEscape) }
  }, [isOpen])

  return (
    <span className={styles.root} ref={rootRef}>
      {label && <span className={styles.prefix}>{label}: </span>}
      <span className={isEdited || source === 'USER' ? styles.edited : styles.ai}>{text}</span>
      {text === 'AI解析' && (
        <button className={styles.info} type="button" aria-label="AI解析について" aria-expanded={isOpen} aria-describedby={isOpen ? tooltipId : undefined} onPointerDown={() => { isPointerFocus.current = true }} onClick={() => { setIsOpen((value) => !value); isPointerFocus.current = false }} onFocus={() => { if (!isPointerFocus.current) setIsOpen(true) }}>
          i
        </button>
      )}
      {isOpen && <span className={styles.tooltip} id={tooltipId} role="tooltip">※「AI解析」は、AIがレシートを解析した結果です。</span>}
    </span>
  )
}
