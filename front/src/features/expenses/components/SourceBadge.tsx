import { InfoTooltip } from '@/shared/ui/InfoTooltip'
import type { ExpenseSource } from '../types/expense.types'
import styles from './SourceBadge.module.css'

type Props = { source: ExpenseSource | undefined; isEdited: boolean; label?: string }

export function SourceBadge({ source, isEdited, label }: Props) {
  const text = isEdited || source === 'USER' ? '編集済み' : source === 'AI' ? 'AI解析' : '由来不明'
  return <span className={styles.root}>
    {label && <span className={styles.prefix}>{label}: </span>}
    {text === 'AI解析' ? <InfoTooltip className={styles.ai} label={text} description="※「AI解析」は、AIがレシートを解析した結果です。" /> : <span className={styles.edited}>{text}</span>}
  </span>
}
