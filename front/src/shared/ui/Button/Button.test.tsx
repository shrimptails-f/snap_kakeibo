import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { Button } from './Button'
import styles from './Button.module.css'

describe('Button', () => {
  it('既定では type="button" で描画し、押すと onClick を呼ぶ', async () => {
    const onClick = vi.fn()
    render(
      <Button variant="secondary" onClick={onClick}>
        再読み込み
      </Button>,
    )

    const button = screen.getByRole('button', { name: '再読み込み' })
    expect(button).toHaveAttribute('type', 'button')
    await userEvent.setup().click(button)
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it('type="submit" を指定するとそのまま反映する', () => {
    render(
      <Button variant="primary" type="submit">
        ログイン
      </Button>,
    )

    expect(screen.getByRole('button', { name: 'ログイン' })).toHaveAttribute('type', 'submit')
  })

  it('variant ごとの class を付け、className は追加で残す', () => {
    render(
      <Button variant="danger" className="wide">
        削除する
      </Button>,
    )

    const button = screen.getByRole('button', { name: '削除する' })
    expect(button).toHaveClass(styles.button, styles.danger, 'wide')
    expect(button).not.toHaveClass(styles.primary)
  })

  it('disabled のときは押しても onClick を呼ばない', async () => {
    const onClick = vi.fn()
    render(
      <Button variant="primary" disabled onClick={onClick}>
        保存中...
      </Button>,
    )

    const button = screen.getByRole('button', { name: '保存中...' })
    expect(button).toBeDisabled()
    await userEvent.setup().click(button)
    expect(onClick).not.toHaveBeenCalled()
  })
})
