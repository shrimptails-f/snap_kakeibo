import type { ReactNode } from 'react'
import styles from './StatusBadge.module.css'

export type StatusTone = 'neutral' | 'info' | 'primary' | 'warning' | 'danger'

type Props = { tone: StatusTone; children: ReactNode }

export function StatusBadge({ tone, children }: Props) {
  return <span className={`${styles.badge} ${styles[tone]}`}>{children}</span>
}
