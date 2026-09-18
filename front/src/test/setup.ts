// 全テスト共通のセットアップ(vite.config.ts の test.setupFiles)
import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

// globals: false で使うので、Testing Library の自動 cleanup を自前で登録する
afterEach(() => {
  cleanup()
})
