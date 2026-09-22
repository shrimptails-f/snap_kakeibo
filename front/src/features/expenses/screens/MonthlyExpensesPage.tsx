import { useEffect, useMemo, useRef, useState, type CSSProperties } from 'react'
import { Link, useLocation, useNavigate, useParams } from 'react-router'
import { formatYen } from '@/shared/lib/formatYen'
import { Button } from '@/shared/ui/Button'
import { SpinnerBlock } from '@/shared/ui/Spinner'
import { useMonthlyExpenses } from '../hooks/useMonthlyExpenses'
import { categoryLabel } from '../lib/categoryLabel'
import { monthlyBreakdown, type CategoryBreakdown } from '../lib/monthlyBreakdown'
import { currentYearMonth, isYearMonth, shiftYearMonth, yearMonthLabel } from '../lib/yearMonth'
import type { MonthlyExpenseItem } from '../types/monthly-expenses.types'
import styles from './MonthlyExpensesPage.module.css'

type RestoreState = { restoreDetailId?: string }

function sourceText(item: MonthlyExpenseItem): string {
  if (item.is_edited) return '編集済み'
  return item.source === 'AI' ? 'AI解析' : '手動登録'
}

function categoryColor(category: string): string {
  return `var(--color-category-${category.replace('_', '-')})`
}

function CategoryChart({ rows }: { rows: CategoryBreakdown[] }) {
  const positiveRows = rows.filter((row) => row.amount > 0)
  const stops = positiveRows.map((row, index) => {
    const start = positiveRows.slice(0, index).reduce((sum, candidate) => sum + (candidate.percentage ?? 0), 0)
    const end = start + (row.percentage ?? 0)
    return `${categoryColor(row.category)} ${start}% ${end}%`
  })
  return <div className={styles.chart} aria-hidden="true" style={{ '--chart-segments': `conic-gradient(${stops.join(', ')})` } as CSSProperties} />
}

function ErrorNotice({ message, action, onRetry }: { message: string; action: string; onRetry: () => void }) {
  return <div className={styles.error} role="alert"><p>{message}</p><Button variant="secondary" onClick={onRetry}>{action}</Button></div>
}

export function MonthlyExpensesPage() {
  const { yearMonth = '' } = useParams()
  const navigate = useNavigate()
  const location = useLocation()
  const [isCoolingDown, setIsCoolingDown] = useState(false)
  const cooldownTimer = useRef<number | undefined>(undefined)
  const validMonth = isYearMonth(yearMonth)
  const { summaries, expenses } = useMonthlyExpenses(yearMonth, validMonth)

  useEffect(() => {
    if (!validMonth) return
    const detailId = (location.state as RestoreState | null)?.restoreDetailId
    if (detailId) requestAnimationFrame(() => document.getElementById(`detail-${detailId}`)?.focus())
    else window.scrollTo({ top: 0 })
  }, [location.key, location.state, validMonth, yearMonth])

  useEffect(() => () => window.clearTimeout(cooldownTimer.current), [])

  const summary = summaries.data?.monthly_summaries.find((value) => value.year_month === yearMonth)
  const items = useMemo(() => expenses.data?.items ?? [], [expenses.data])
  const breakdown = useMemo(() => monthlyBreakdown(items), [items])
  const hasNegativeDetail = items.some((item) => item.amount < 0)
  const isFetching = summaries.isFetching || expenses.isFetching
  const bothFailed = summaries.isError && !summaries.data && expenses.isError && !expenses.data
  const isEmpty = summaries.isSuccess && items.length === 0 && (!summary || (summary.total_recorded_amount === 0 && summary.expense_count === 0 && summary.detail_count === 0))
  const isInconsistent = Boolean((items.length > 0 && !summary && summaries.isSuccess) || (summary && summary.detail_count !== items.length))

  function moveTo(month: string) {
    if (isYearMonth(month)) navigate(`/months/${month}`)
  }

  async function reloadAll() {
    setIsCoolingDown(true)
    window.clearTimeout(cooldownTimer.current)
    cooldownTimer.current = window.setTimeout(() => setIsCoolingDown(false), 5000)
    await Promise.allSettled([summaries.refetch(), expenses.refetch()])
  }

  if (!validMonth) {
    return <section className={styles.invalid}><h1>指定された月を表示できません</h1><p>URLの年月を確認するか、今月を開いてください。</p><Link to={`/months/${currentYearMonth()}`}>今月を開く</Link></section>
  }

  if (summaries.isPending && expenses.isPending) return <SpinnerBlock label="月別支出を読み込み中" />

  const previous = shiftYearMonth(yearMonth, -1)
  const next = shiftYearMonth(yearMonth, 1)
  const current = currentYearMonth()
  const maxAmount = Math.max(0, ...items.map((item) => item.amount))

  return (
    <div className={styles.page}>
      <header className={styles.pageHeader}>
        <div><h1>月別支出</h1><p>{yearMonthLabel(yearMonth)}に購入した支出</p></div>
        <Button variant="secondary" disabled={isFetching || isCoolingDown} onClick={reloadAll}>{isFetching ? '再読み込み中…' : '再読み込み'}</Button>
      </header>

      <nav className={styles.monthNav} aria-label="表示月">
        <Link to={`/months/${previous}`} aria-label={`${yearMonthLabel(previous)}を表示`}>‹ 前月</Link>
        <label>表示する月<input type="month" value={yearMonth} onChange={(event) => moveTo(event.target.value)} /></label>
        <Link to={`/months/${next}`} aria-label={`${yearMonthLabel(next)}を表示`}>翌月 ›</Link>
        <Button variant="secondary" disabled={yearMonth === current} onClick={() => moveTo(current)}>今月</Button>
      </nav>

      {(summaries.isRefetchError || expenses.isRefetchError) && (summary || items.length > 0) && <p className={styles.warning} role="alert">更新できませんでした。前回取得した内容です。</p>}

      {bothFailed ? (
        <ErrorNotice message="月別支出を読み込めませんでした。" action="再読み込み" onRetry={() => void reloadAll()} />
      ) : summaries.isError && !summaries.data ? (
        <ErrorNotice message="月合計を取得できませんでした。" action="月合計を再読み込み" onRetry={() => void summaries.refetch()} />
      ) : summary ? (
        <section className={styles.total} aria-labelledby="monthly-total-heading">
          <h2 id="monthly-total-heading">月合計（計上額）</h2>
          <strong className="amount">{formatYen(summary.total_recorded_amount)}</strong>
          <p>支出 {summary.expense_count}件 / 明細 {summary.detail_count}件</p>
          {summary.total_recorded_amount < 0 && <p>返金などの調整により、今月の計上額はマイナスです。</p>}
        </section>
      ) : null}

      {!bothFailed && (expenses.isError && !expenses.data ? (
        <ErrorNotice message="明細を読み込めませんでした。" action="明細を再読み込み" onRetry={() => void expenses.refetch()} />
      ) : expenses.isPending ? <SpinnerBlock label="支出明細を読み込み中" /> : isEmpty ? (
        <section className={styles.empty}><h2>この月の支出はまだありません</h2><p>レシートを取り込むと、ここで月ごとの支出を確認できます。</p><Link to="/">レシートを取り込む ›</Link></section>
      ) : (
        <>
          {isInconsistent && <div className={styles.warning} role="alert"><p>月合計と明細の情報が揃っていません。</p><Button variant="secondary" onClick={reloadAll}>再読み込み</Button></div>}
          <section className={styles.breakdown} aria-labelledby="category-heading">
            <div className={styles.sectionHeading}><div><h2 id="category-heading">カテゴリ別内訳</h2><p>明細合計 <strong className="amount">{formatYen(breakdown.total)}</strong></p></div></div>
            <div className={styles.breakdownGrid}>
              {breakdown.canDrawChart ? <CategoryChart rows={breakdown.rows} /> : <p className={styles.noChart}>{hasNegativeDetail ? '負の明細金額があるため、円グラフと割合は表示しません。' : '明細金額の合計は0円です。'}</p>}
              <table><thead><tr><th scope="col">カテゴリ</th><th scope="col">明細金額</th><th scope="col">割合</th></tr></thead><tbody>{breakdown.rows.map((row) => <tr key={row.category}><th scope="row"><span className={styles.swatch} data-category={row.category} />{categoryLabel(row.category)}</th><td className="amount">{formatYen(row.amount)}</td><td>{row.percentage === null ? '—' : `${row.percentage.toFixed(1)}%`}</td></tr>)}</tbody></table>
            </div>
            <p className={styles.help}>内訳は明細金額に基づきます。調整額や読取金額との差により、月合計と一致しない場合があります。AIによるカテゴリは参考値です。</p>
          </section>

          <section className={styles.details} aria-labelledby="monthly-details-heading">
            <div className={styles.sectionHeading}><h2 id="monthly-details-heading">支出明細 <span>{items.length}件</span></h2><p>金額の大きい順</p></div>
            <ol>{items.map((item) => {
              const [,, day] = item.purchase_date.split('-').map(Number)
              const width = maxAmount > 0 && item.amount > 0 ? `${Math.max(2, item.amount / maxAmount * 100)}%` : '0%'
              return <li key={item.detail_id} id={`detail-${item.detail_id}`} tabIndex={-1}><Link to={`/expenses/${item.expense_id}`} state={{ from: `/months/${yearMonth}`, backLabel: `${yearMonthLabel(yearMonth)}の支出へ`, detailId: item.detail_id, scrollY: window.scrollY }}><span className={styles.itemTop}><strong>{item.name}</strong><strong className="amount">{formatYen(item.amount)} <span aria-hidden="true">›</span></strong></span>{!hasNegativeDetail && <span className={styles.barTrack} aria-hidden="true"><span style={{ width }} /></span>}<span className={styles.itemMeta}>{Number(yearMonth.slice(5))}/{day} · {item.store_name}</span><span className={styles.itemMeta}>{categoryLabel(item.category)} · 数量{item.quantity} · {sourceText(item)}</span></Link></li>
            })}</ol>
          </section>
        </>
      ))}
    </div>
  )
}
