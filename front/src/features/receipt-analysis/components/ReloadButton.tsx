import type { CSSProperties } from 'react'
import { Button } from '@/shared/ui/Button'
import { RELOAD_COOLDOWN_MS } from '../hooks/useReloadAnalysisRequests'
import type { ReloadAnalysisRequests } from '../hooks/useReloadAnalysisRequests'
import styles from './ReloadButton.module.css'

type Props = {
  reload: ReloadAnalysisRequests
  idleLabel?: string
  fetchingLabel?: string
}

const cooldownStyle = { '--cooldown-duration': `${RELOAD_COOLDOWN_MS}ms` } as CSSProperties

export function ReloadButton({ reload, idleLabel = '再読み込み', fetchingLabel = '再読み込み中…' }: Props) {
  const remainingSeconds = Math.max(1, Math.ceil(reload.cooldownRemainingMs / 1000))
  const isCountingDown = reload.isCoolingDown && !reload.isFetching
  const accessibleLabel = isCountingDown ? `${idleLabel}（あと${remainingSeconds}秒）` : undefined
  const classes = [styles.reloadButton, reload.isCoolingDown ? styles.coolingDown : ''].filter(Boolean).join(' ')

  return (
    <Button
      variant="secondary"
      className={classes}
      style={cooldownStyle}
      onClick={reload.reload}
      disabled={reload.isDisabled}
      aria-label={accessibleLabel}
    >
      <span className={styles.label}>{reload.isFetching ? fetchingLabel : idleLabel}</span>
      {isCountingDown && <span className={styles.countdown} aria-hidden="true">{remainingSeconds}秒</span>}
    </Button>
  )
}
