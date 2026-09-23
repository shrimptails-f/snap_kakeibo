import { useEffect, useMemo, type CSSProperties } from 'react'
import { Link, useLocation, useNavigate, useParams } from 'react-router'
import { formatYen } from '@/shared/lib/formatYen'
import { useReloadCooldown } from '@/shared/hooks/useReloadCooldown'
import { Button } from '@/shared/ui/Button'
import { ReloadButton, type ReloadControl } from '@/shared/ui/ReloadButton'
import { SpinnerBlock } from '@/shared/ui/Spinner'
import { useMonthlyExpenses } from '../hooks/useMonthlyExpenses'
import { categoryLabel } from '../lib/categoryLabel'
import { formatUpdatedAt } from '../lib/formatUpdatedAt'
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

function ErrorNotice({ message, action, reload }: { message: string; action: string; reload: ReloadControl }) {
  return <div className={styles.error} role="alert"><p>{message}</p><ReloadButton reload={reload} idleLabel={action} /></div>
}

export function MonthlyExpensesPage() {
  const { yearMonth = '' } = useParams()
  const navigate = useNavigate()
  const location = useLocation()
  const cooldown = useReloadCooldown()
  const validMonth = isYearMonth(yearMonth)
  const { summaries, expenses } = useMonthlyExpenses(yearMonth, validMonth)

  useEffect(() => {
    if (!validMonth) return
    const detailId = (location.state as RestoreState | null)?.restoreDetailId
    if (detailId) requestAnimationFrame(() => document.getElementById(`detail-${detailId}`)?.focus())
    else window.scrollTo({ top: 0 })
  }, [location.key, location.state, validMonth, yearMonth])

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

  function reloadAll() {
    if (!cooldown.startCooldown()) return
    void Promise.allSettled([summaries.refetch(), expenses.refetch()])
  }

  function reloadSummaries() {
    if (!cooldown.startCooldown()) return
    void summaries.refetch()
  }

  function reloadExpenses() {
    if (!cooldown.startCooldown()) return
    void expenses.refetch()
  }

  function reloadControl(reload: () => void): ReloadControl {
    return {
      reload,
      isDisabled: isFetching || cooldown.isCoolingDown,
      isFetching,
      isCoolingDown: cooldown.isCoolingDown,
      cooldownRemainingMs: cooldown.cooldownRemainingMs,
    }
  }

  if (!validMonth) {
    return <section className={styles.invalid}><h1>指定された月を表示できません</h1><p>URLの年月を確認するか、今月を開いてください。</p><Link to={`/months/${currentYearMonth()}`}>今月を開く</Link></section>
  }

  if (summaries.isPending && expenses.isPending) return <SpinnerBlock label="月別支出を読み込み中" />

  const previous = shiftYearMonth(yearMonth, -1)
  const next = shiftYearMonth(yearMonth, 1)
  const maxAmount = Math.max(0, ...items.map((item) => item.tax_included_amount ?? item.amount))

  return (
    <div className={styles.page}>
      <header className={styles.pageHeader}>
        <h1>月別支出</h1>
        <ReloadButton reload={reloadControl(reloadAll)} />
      </header>

      <nav className={styles.monthNav} aria-label="表示月">
        <div className={styles.monthSwitcher}>
          <Button variant="secondary" aria-label={`${yearMonthLabel(previous)}を表示`} onClick={() => moveTo(previous)}>‹ 前月</Button>
          <label>表示する月<input className="field-control" type="month" value={yearMonth} onChange={(event) => moveTo(event.target.value)} /></label>
          <Button variant="secondary" aria-label={`${yearMonthLabel(next)}を表示`} onClick={() => moveTo(next)}>翌月 ›</Button>
        </div>
      </nav>

      {(summaries.isRefetchError || expenses.isRefetchError) && (summary || items.length > 0) && <p className={styles.warning} role="alert">更新できませんでした。前回取得した内容です。</p>}

      {bothFailed ? (
        <ErrorNotice message="月別支出を読み込めませんでした。" action="再読み込み" reload={reloadControl(reloadAll)} />
      ) : summaries.isError && !summaries.data ? (
        <ErrorNotice message="月合計を取得できませんでした。" action="月合計を再読み込み" reload={reloadControl(reloadSummaries)} />
      ) : summary ? (
        <section className={styles.total} aria-labelledby="monthly-total-heading">
          <div className={styles.totalRow}><h2 id="monthly-total-heading">月合計（計上額）</h2><strong className="amount">{formatYen(summary.total_recorded_amount)}</strong></div>
          <p>支出 {summary.expense_count}件 / 明細 {summary.detail_count}件</p>
          <p>集計更新：{formatUpdatedAt(summary.updated_at)}</p>
          {summary.total_recorded_amount < 0 && <p>返金などの調整により、今月の計上額はマイナスです。</p>}
        </section>
      ) : null}

      {!bothFailed && (expenses.isError && !expenses.data ? (
        <ErrorNotice message="明細を読み込めませんでした。" action="明細を再読み込み" reload={reloadControl(reloadExpenses)} />
      ) : expenses.isPending ? <SpinnerBlock label="支出明細を読み込み中" /> : isEmpty ? (
        <section className={styles.empty}><h2>この月の支出はまだありません</h2><p>レシートを取り込むと、ここで月ごとの支出を確認できます。</p><Link to="/upload">レシートを取り込む ›</Link></section>
      ) : (
        <>
          {isInconsistent && <div className={styles.warning} role="alert"><p>月合計と明細の情報が揃っていません。</p><ReloadButton reload={reloadControl(reloadAll)} /></div>}
          <section className={styles.breakdown} aria-labelledby="category-heading">
            <div className={styles.sectionHeading}><div><h2 id="category-heading">カテゴリ別内訳</h2><p>明細合計 <strong className="amount">{formatYen(breakdown.total)}</strong></p></div></div>
            {breakdown.unconfirmedCount > 0 && <p className={styles.help}>税込み額未確定の明細 {breakdown.unconfirmedCount}件は印字額で含めています。円グラフと割合は暫定値です。</p>}
            <div className={styles.breakdownGrid}>
              {breakdown.canDrawChart ? <CategoryChart rows={breakdown.rows} /> : <p className={styles.noChart}>{hasNegativeDetail ? '負の明細金額があるため、円グラフと割合は表示しません。' : '明細金額の合計は0円です。'}</p>}
              <table><thead><tr><th scope="col">カテゴリ</th><th scope="col">明細金額</th><th scope="col">割合</th></tr></thead><tbody>{breakdown.rows.map((row) => <tr key={row.category}><th scope="row"><span className={styles.swatch} data-category={row.category} />{categoryLabel(row.category)}</th><td className="amount">{formatYen(row.amount)}</td><td>{row.percentage === null ? '—' : `${row.percentage.toFixed(1)}%`}</td></tr>)}</tbody></table>
            </div>
            <p className={styles.help}>内訳は確定した税込み明細額を使い、未確定の行は印字額を使います。店舗の値引き、利用者の調整、読取漏れにより月合計と一致しない場合があります。AIによるカテゴリは参考値です。</p>
          </section>

          <section className={styles.details} aria-labelledby="monthly-details-heading">
            <div className={styles.sectionHeading}><h2 id="monthly-details-heading">支出明細 <span>{items.length}件</span></h2><p>金額の大きい順</p></div>
            <ol>{items.map((item) => {
              const [,, day] = item.purchase_date.split('-').map(Number)
              const shownAmount = item.tax_included_amount ?? item.amount
              const width = maxAmount > 0 && shownAmount > 0 ? `${Math.max(2, shownAmount / maxAmount * 100)}%` : '0%'
              return <li key={item.detail_id} id={`detail-${item.detail_id}`} tabIndex={-1}><Link to={`/expenses/${item.expense_id}`} state={{ from: `/months/${yearMonth}`, backLabel: `${yearMonthLabel(yearMonth)}の支出へ`, detailId: item.detail_id, scrollY: window.scrollY }}><span className={styles.itemTop}><strong>{item.name}</strong><strong className="amount">{formatYen(shownAmount)} <span aria-hidden="true">›</span></strong></span>{!hasNegativeDetail && <span className={styles.barTrack} aria-hidden="true"><span style={{ width, background: categoryColor(item.category) }} /></span>}<span className={styles.itemMeta}>{item.tax_included_amount === undefined ? '印字額・税込み未確定' : `税込み明細額（印字額 ${formatYen(item.amount)}）`}{item.tax_rate !== undefined ? ` · 税率 ${item.tax_rate}%` : ''}{item.tax_mode === 'external' && item.tax_included_amount !== undefined ? ` · 配分税額 ${formatYen(item.tax_included_amount - item.amount)}` : ''}</span><span className={styles.itemMeta}>{Number(yearMonth.slice(5))}/{day} · {item.store_name}</span><span className={styles.itemMeta}>{categoryLabel(item.category)} · 数量{item.quantity} · {sourceText(item)}</span></Link></li>
            })}</ol>
          </section>
        </>
      ))}
    </div>
  )
}
