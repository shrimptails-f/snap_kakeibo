import type { ButtonHTMLAttributes } from 'react'
import styles from './Button.module.css'

// design_guidelines.md §5 の役割。primary は画面または領域内で原則 1 つ
export type ButtonVariant = 'primary' | 'secondary' | 'danger'

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant: ButtonVariant
}

const VARIANT_CLASS: Record<ButtonVariant, string> = {
  primary: styles.primary,
  secondary: styles.secondary,
  danger: styles.danger,
}

// 操作を実行するボタン。遷移は Link を使う(coding_rules.md §8)。
// 進行中の表示(「保存中...」)は呼び出し側が children と disabled で表す。
// className は幅や配置など置き場所の都合だけに使い、色や形は variant で選ぶ
export function Button({ variant, type = 'button', className, ...rest }: Props) {
  const classes = [styles.button, VARIANT_CLASS[variant], className].filter(Boolean).join(' ')
  return <button className={classes} type={type} {...rest} />
}
