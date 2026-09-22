import { Suspense } from 'react'
import { Link, useParams, useSearchParams } from 'react-router'
import { useQueryErrorResetBoundary } from '@tanstack/react-query'
import { isApiError } from '@/shared/api/client'
import { Button } from '@/shared/ui/Button'
import { ErrorBoundary } from '@/shared/ui/ErrorBoundary'
import { SpinnerBlock } from '@/shared/ui/Spinner'
import { ExpenseDetailContent } from '../components/ExpenseDetailContent'
import styles from './ExpenseDetailPage.module.css'

export function ExpenseDetailPage() {
  const { expenseId = '' } = useParams()
  const [searchParams] = useSearchParams()
  const isFromUpload = searchParams.get('from') === 'upload'
  const requestId = searchParams.get('request')
  const backTo = isFromUpload ? `/upload${requestId ? `?request=${encodeURIComponent(requestId)}` : ''}` : '/analysis-requests'
  const backLabel = isFromUpload ? 'アップロードへ' : '解析履歴へ'
  const resetBoundary = useQueryErrorResetBoundary()
  return <ErrorBoundary onReset={resetBoundary.reset} fallback={(error, reset) => <div className={styles.error}><Link to={backTo}>&lt; {backLabel}</Link><h1>{isApiError(error) && error.status === 404 ? '支出が見つかりません' : '支出を読み込めませんでした'}</h1><p>時間をおいて、もう一度お試しください。</p><Button variant="secondary" onClick={reset}>再読み込み</Button></div>}><Suspense fallback={<div className={styles.loading}><Link to={backTo}>&lt; {backLabel}</Link><SpinnerBlock label="支出を読み込み中" /></div>}><ExpenseDetailContent expenseId={expenseId} /></Suspense></ErrorBoundary>
}
