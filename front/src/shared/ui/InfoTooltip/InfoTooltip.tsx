import { useEffect, useId, useRef, useState, type ReactNode } from 'react'
import styles from './InfoTooltip.module.css'

type Props = { label: ReactNode; description: string; className?: string }

export function InfoTooltip({ label, description, className }: Props) {
  const [isOpen, setIsOpen] = useState(false)
  const rootRef = useRef<HTMLSpanElement>(null)
  const isPointerFocus = useRef(false)
  const isPinned = useRef(false)
  const hoverTimer = useRef<number | undefined>(undefined)
  const tooltipId = useId()

  function close() {
    window.clearTimeout(hoverTimer.current)
    isPinned.current = false
    setIsOpen(false)
  }

  useEffect(() => {
    if (!isOpen) return
    const closeOutside = (event: PointerEvent) => { if (!rootRef.current?.contains(event.target as Node)) close() }
    const closeEscape = (event: KeyboardEvent) => { if (event.key === 'Escape') close() }
    const closeScroll = () => close()
    document.addEventListener('pointerdown', closeOutside)
    document.addEventListener('keydown', closeEscape)
    window.addEventListener('scroll', closeScroll, true)
    return () => {
      document.removeEventListener('pointerdown', closeOutside)
      document.removeEventListener('keydown', closeEscape)
      window.removeEventListener('scroll', closeScroll, true)
    }
  }, [isOpen])

  useEffect(() => () => window.clearTimeout(hoverTimer.current), [])

  return <span className={styles.root} ref={rootRef} onPointerEnter={(event) => { if (event.pointerType === 'mouse') hoverTimer.current = window.setTimeout(() => setIsOpen(true), 250) }} onPointerLeave={() => { window.clearTimeout(hoverTimer.current); if (!isPinned.current) setIsOpen(false) }}>
    <button className={`${styles.trigger} ${className ?? ''}`} type="button" aria-expanded={isOpen} aria-describedby={isOpen ? tooltipId : undefined} onPointerDown={() => { isPointerFocus.current = true }} onClick={() => { if (isPinned.current) close(); else { isPinned.current = true; setIsOpen(true) } isPointerFocus.current = false }} onFocus={() => { if (!isPointerFocus.current) setIsOpen(true) }} onBlur={(event) => { if (!rootRef.current?.contains(event.relatedTarget)) close() }}>{label}</button>
    {isOpen && <span className={styles.tooltip} id={tooltipId} role="tooltip">{description}</span>}
  </span>
}
