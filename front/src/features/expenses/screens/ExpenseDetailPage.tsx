import { Suspense } from 'react'
import { Link, useLocation, useParams } from 'react-router'
import { useQueryErrorResetBoundary } from '@tanstack/react-query'
import { isApiError } from '@/shared/api/client'
import { Button } from '@/shared/ui/Button'
import { ErrorBoundary } from '@/shared/ui/ErrorBoundary'
import { SpinnerBlock } from '@/shared/ui/Spinner'
import { ExpenseDetailContent } from '../components/ExpenseDetailContent'
import { expenseBackLink, type ExpenseLocationState } from '../lib/expenseBackLink'
import styles from './ExpenseDetailPage.module.css'

export function ExpenseDetailPage() {
  const { expenseId = '' } = useParams()
  const location = useLocation()
  const back = expenseBackLink(location.search, location.state as ExpenseLocationState | null)
  const resetBoundary = useQueryErrorResetBoundary()
  return <ErrorBoundary onReset={resetBoundary.reset} fallback={(error, reset) => <div className={styles.error}><Link to={back.to} state={back.state}>&lt; {back.label}</Link><h1>{isApiError(error) && error.status === 404 ? '支出が見つかりません' : '支出を読み込めませんでした'}</h1><p>時間をおいて、もう一度お試しください。</p><Button variant="secondary" onClick={reset}>再読み込み</Button></div>}><Suspense fallback={<div className={styles.loading}><Link to={back.to} state={back.state}>&lt; {back.label}</Link><SpinnerBlock label="支出を読み込み中" /></div>}><ExpenseDetailContent expenseId={expenseId} /></Suspense></ErrorBoundary>
}
