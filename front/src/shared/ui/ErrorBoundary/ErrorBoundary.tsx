import { Component } from 'react'
import type { ReactNode } from 'react'

type Props = {
  // 失敗時の表示。reset を呼ぶと children を描き直す
  fallback: (error: unknown, reset: () => void) => ReactNode
  // reset の直前に呼ぶ(Query の error 状態を消すなど)
  onReset?: () => void
  children: ReactNode
}

type State = {
  error: unknown
  hasError: boolean
}

// 描画中の例外(useSuspenseQuery の取得失敗など)を領域単位で受ける。
// 画面全体は Router の errorElement が受けるので、パネルのように一部だけ回復させたい場所で使う
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null, hasError: false }

  static getDerivedStateFromError(error: unknown): State {
    return { error, hasError: true }
  }

  reset = (): void => {
    this.props.onReset?.()
    this.setState({ error: null, hasError: false })
  }

  render(): ReactNode {
    if (this.state.hasError) {
      return this.props.fallback(this.state.error, this.reset)
    }
    return this.props.children
  }
}
