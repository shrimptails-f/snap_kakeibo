import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { ReloadControl } from './ReloadButton'
import { ReloadButton } from './ReloadButton'
import styles from './ReloadButton.module.css'

function reloadState(overrides: Partial<ReloadControl> = {}): ReloadControl {
  return {
    reload: vi.fn(),
    isDisabled: false,
    isFetching: false,
    isCoolingDown: false,
    cooldownRemainingMs: 0,
    ...overrides,
  }
}

describe('ReloadButton', () => {
  it('クールタイム中は残り秒数とチャージ表示を示して無効にする', () => {
    render(<ReloadButton reload={reloadState({ isDisabled: true, isCoolingDown: true, cooldownRemainingMs: 4200 })} />)

    const button = screen.getByRole('button', { name: '再読み込み（あと5秒）' })
    expect(button).toBeDisabled()
    expect(button).toHaveTextContent('再読み込み5秒')
    expect(button).toHaveClass(styles.coolingDown)
  })

  it('取得中は進行中の文言を優先する', () => {
    render(<ReloadButton reload={reloadState({ isDisabled: true, isFetching: true, isCoolingDown: true, cooldownRemainingMs: 5000 })} />)

    expect(screen.getByRole('button', { name: '再読み込み中…' })).toBeDisabled()
    expect(screen.queryByText('5秒')).not.toBeInTheDocument()
  })
})
