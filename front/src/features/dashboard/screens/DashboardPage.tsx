import { useEffect, useMemo, useRef, useState, type CSSProperties } from 'react'
import { Link, useSearchParams } from 'react-router'
import { formatYen } from '@/shared/lib/formatYen'
import { Button } from '@/shared/ui/Button'
import { SpinnerBlock } from '@/shared/ui/Spinner'
import { useMonthlySummaries } from '../hooks/useMonthlySummaries'
import {
  CATEGORY_LABELS,
  categoryRows,
  currentYearMonth,
  detailTotal,
  formatUpdatedAt,
  monthsBetween,
  monthsEndingAt,
  shiftYearMonth,
  shortMonthLabel,
  yearMonthLabel,
} from '../lib/dashboardData'
import { dashboardCategories } from '../types/monthly-summary.schema'
import type { MonthlySummary } from '../types/monthly-summary.types'
import styles from './DashboardPage.module.css'

type ChartMode = 'category' | 'recorded'
type ChartStyle = CSSProperties & Record<'--segment-start' | '--segment-size', string>

function valueStyle(start: number, size: number): ChartStyle {
  return { '--segment-start': `${start}%`, '--segment-size': `${size}%` }
}

function signedYen(value: number): string {
  if (value === 0) return '±0円'
  return `${value > 0 ? '+' : ''}${formatYen(value)}`
}

function previousMonthChange(summary: MonthlySummary, summariesByMonth: Map<string, MonthlySummary>): string {
  const previous = summariesByMonth.get(shiftYearMonth(summary.year_month, -1))
  return previous ? `計上額の前月比 ${signedYen(summary.total_recorded_amount - previous.total_recorded_amount)}` : '計上額の前月比較なし'
}

function graphScale(period: string[], summariesByMonth: Map<string, MonthlySummary>, mode: ChartMode) {
  let maximum = 0
  let minimum = 0
  for (const month of period) {
    const summary = summariesByMonth.get(month)
    if (!summary) continue
    if (mode === 'recorded') {
      maximum = Math.max(maximum, summary.total_recorded_amount)
      minimum = Math.min(minimum, summary.total_recorded_amount)
      continue
    }
    const amounts = dashboardCategories.map((category) => summary.category_totals[category])
    maximum = Math.max(maximum, amounts.filter((amount) => amount > 0).reduce((sum, amount) => sum + amount, 0))
    minimum = Math.min(minimum, amounts.filter((amount) => amount < 0).reduce((sum, amount) => sum + amount, 0))
  }
  const range = maximum - minimum || 1
  return { maximum, minimum, zero: ((0 - minimum) / range) * 100, range }
}

function segmentPositions(summary: MonthlySummary, minimum: number, range: number) {
  let positive = 0
  let negative = 0
  return dashboardCategories.flatMap((category) => {
    const amount = summary.category_totals[category]
    if (amount === 0) return []
    const low = amount > 0 ? positive : negative + amount
    if (amount > 0) positive += amount
    else negative += amount
    return [{ category, amount, start: ((low - minimum) / range) * 100, size: (Math.abs(amount) / range) * 100 }]
  })
}

function SummaryGraph({
  mode,
  period,
  summariesByMonth,
  onOpenMonth,
}: {
  mode: ChartMode
  period: string[]
  summariesByMonth: Map<string, MonthlySummary>
  onOpenMonth: () => void
}) {
  const scale = graphScale(period, summariesByMonth, mode)
  return (
    <div className={styles.graph} style={{ '--zero-position': `${scale.zero}%` } as CSSProperties}>
      <div className={styles.axis} aria-hidden="true">
        {scale.maximum > 0 && <span>{formatYen(scale.maximum)}</span>}
        <span>0円</span>
        {scale.minimum < 0 && <span>{formatYen(scale.minimum)}</span>}
      </div>
      <ol className={styles.months} aria-label={mode === 'category' ? '月ごとのカテゴリ別明細合計' : '月ごとの計上額'}>
        {period.map((month, index) => {
          const summary = summariesByMonth.get(month)
          const previousMonth = index === 0 ? undefined : period[index - 1]
          const visibleAmount = summary ? (mode === 'category' ? detailTotal(summary) : summary.total_recorded_amount) : null
          return (
            <li className={styles.month} key={month}>
              <div className={styles.plot}>
                <span className={styles.zeroLine} aria-hidden="true" />
                {!summary ? <span className={styles.noBar}>集計なし</span> : mode === 'category' ? (
                  segmentPositions(summary, scale.minimum, scale.range).map((segment) => (
                    <span
                      key={segment.category}
                      className={styles.segment}
                      data-category={segment.category}
                      style={valueStyle(segment.start, segment.size)}
                      aria-hidden="true"
                    />
                  ))
                ) : (
                  <Link
                    className={styles.recordedLink}
                    to={`/months/${month}`}
                    onClick={onOpenMonth}
                    aria-label={`${yearMonthLabel(month)}、計上額${formatYen(summary.total_recorded_amount)}、月別支出を見る`}
                  >
                    <span
                      className={styles.recordedBar}
                      style={valueStyle(
                        ((Math.min(0, summary.total_recorded_amount) - scale.minimum) / scale.range) * 100,
                        (Math.abs(summary.total_recorded_amount) / scale.range) * 100,
                      )}
                    />
                  </Link>
                )}
              </div>
              <Link className={styles.monthLink} to={`/months/${month}`} onClick={onOpenMonth}>{shortMonthLabel(month, previousMonth)}</Link>
              <strong className={`amount ${styles.graphValue}`}>{visibleAmount === null ? '—（集計なし）' : formatYen(visibleAmount)}</strong>
              {summary && <small>{previousMonthChange(summary, summariesByMonth)}</small>}
            </li>
          )
        })}
      </ol>
    </div>
  )
}

function InitialError({ onRetry }: { onRetry: () => void }) {
  return (
    <section className={styles.error} role="alert">
      <h2>月ごとの支出を取得できませんでした</h2>
      <p>通信状態を確認して、もう一度お試しください。</p>
      <Button variant="secondary" onClick={onRetry}>再試行</Button>
    </section>
  )
}

export function DashboardPage() {
  const query = useMonthlySummaries()
  const [searchParams, setSearchParams] = useSearchParams()
  const [isCoolingDown, setIsCoolingDown] = useState(false)
  const cooldownTimer = useRef<number | undefined>(undefined)
  const didRestoreScroll = useRef(false)
  const scrollStorageKey = `dashboard-scroll:${searchParams.toString()}`
  const nowMonth = currentYearMonth()
  const summaries = useMemo(
    () => [...(query.data?.monthly_summaries ?? [])].sort((left, right) => right.year_month.localeCompare(left.year_month)),
    [query.data],
  )
  const summariesByMonth = useMemo(() => new Map(summaries.map((summary) => [summary.year_month, summary])), [summaries])

  useEffect(() => () => window.clearTimeout(cooldownTimer.current), [])

  useEffect(() => {
    if (!query.isSuccess || didRestoreScroll.current) return
    didRestoreScroll.current = true
    const storedPosition = window.sessionStorage.getItem(scrollStorageKey)
    if (storedPosition === null) return
    window.sessionStorage.removeItem(scrollStorageKey)
    const scrollPosition = Number(storedPosition)
    if (Number.isFinite(scrollPosition)) requestAnimationFrame(() => window.scrollTo({ top: scrollPosition }))
  }, [query.isSuccess, scrollStorageKey])

  if (query.isPending) return <SpinnerBlock label="月ごとの支出を読み込み中" />

  const requestedMode = searchParams.get('view')
  const mode: ChartMode = requestedMode === 'recorded' ? 'recorded' : 'category'
  const newestMonth = summaries[0]?.year_month
  const oldestMonth = summaries.at(-1)?.year_month
  const latestSelectableMonth = newestMonth && newestMonth > nowMonth ? newestMonth : nowMonth
  const earliestSelectableMonth = oldestMonth && oldestMonth < nowMonth ? oldestMonth : nowMonth
  const requestedEndMonth = searchParams.get('end')
  const endMonth = requestedEndMonth && /^\d{4}-(0[1-9]|1[0-2])$/.test(requestedEndMonth)
    && requestedEndMonth >= (oldestMonth ?? nowMonth) && requestedEndMonth <= latestSelectableMonth
    ? requestedEndMonth : nowMonth
  const period = monthsEndingAt(endMonth)
  const referenceCandidates = period.filter((month) => summariesByMonth.has(month))
  const requestedReferenceMonth = searchParams.get('reference')
  const referenceMonth = requestedReferenceMonth && period.includes(requestedReferenceMonth)
    ? requestedReferenceMonth : (referenceCandidates.at(-1) ?? endMonth)
  const referenceSummary = summariesByMonth.get(referenceMonth)
  const selectableMonths = monthsBetween(earliestSelectableMonth, latestSelectableMonth)
  const canMovePrevious = Boolean(oldestMonth && shiftYearMonth(endMonth, -6) >= oldestMonth)
  const canMoveNext = endMonth < latestSelectableMonth

  function updateParams(values: { end?: string; reference?: string; mode?: ChartMode }) {
    const next = new URLSearchParams(searchParams)
    if (values.end) next.set('end', values.end)
    if (values.reference) next.set('reference', values.reference)
    if (values.mode) next.set('view', values.mode)
    setSearchParams(next, { replace: true })
  }

  function selectEndMonth(nextEndMonth: string) {
    const nextPeriod = monthsEndingAt(nextEndMonth)
    const nextReference = nextPeriod.includes(referenceMonth)
      ? referenceMonth
      : ([...nextPeriod].reverse().find((month) => summariesByMonth.has(month)) ?? nextEndMonth)
    updateParams({ end: nextEndMonth, reference: nextReference })
  }

  function rememberScrollPosition() {
    window.sessionStorage.setItem(scrollStorageKey, String(window.scrollY))
  }

  async function reload() {
    setIsCoolingDown(true)
    window.clearTimeout(cooldownTimer.current)
    cooldownTimer.current = window.setTimeout(() => setIsCoolingDown(false), 5000)
    await query.refetch()
  }

  if (query.isError && !query.data) {
    return (
      <div className={styles.page}>
        <header className={styles.pageHeader}><div><h1>ダッシュボード</h1><p>月ごとの支出を振り返る</p></div></header>
        <InitialError onRetry={() => void reload()} />
      </div>
    )
  }

  if (summaries.length === 0) {
    return (
      <div className={styles.page}>
        <header className={styles.pageHeader}><div><h1>ダッシュボード</h1><p>月ごとの支出を振り返る</p></div></header>
        <section className={styles.empty}>
          <h2>まだ月ごとの集計がありません</h2>
          <p>レシートを取り込むと、登録された支出を月ごとに確認できます。</p>
          <div className={styles.emptyActions}><Link to="/upload">レシートを取り込む ›</Link><Link to="/analysis-requests">取り込み済みの場合は、解析履歴を確認 ›</Link></div>
          <Button variant="secondary" disabled={query.isFetching || isCoolingDown} onClick={() => void reload()}>{query.isFetching ? '再読み込み中…' : '再読み込み'}</Button>
        </section>
      </div>
    )
  }

  const referenceRows = referenceSummary ? categoryRows(referenceSummary) : []
  const previousReference = referenceSummary ? summariesByMonth.get(shiftYearMonth(referenceSummary.year_month, -1)) : undefined

  return (
    <div className={styles.page}>
      <header className={styles.pageHeader}>
        <div><h1>ダッシュボード</h1><p>月ごとの支出を振り返る</p></div>
        <Link className={styles.uploadLink} to="/upload">レシートを取り込む ›</Link>
      </header>

      <section className={styles.periodControls} aria-labelledby="period-heading">
        <h2 id="period-heading" className={styles.visuallyHidden}>表示期間</h2>
        <label>表示終了月
          <select value={endMonth} onChange={(event) => selectEndMonth(event.target.value)}>
            {selectableMonths.map((month) => <option key={month} value={month}>{yearMonthLabel(month)}{month > nowMonth ? '（未来月）' : ''}</option>)}
          </select>
        </label>
        <div className={styles.periodButtons}>
          <Button variant="secondary" disabled={!canMovePrevious} onClick={() => selectEndMonth(shiftYearMonth(endMonth, -6))}>‹ 前の6か月</Button>
          <Button variant="secondary" disabled={!canMoveNext} onClick={() => selectEndMonth(shiftYearMonth(endMonth, 6) > latestSelectableMonth ? latestSelectableMonth : shiftYearMonth(endMonth, 6))}>次の6か月 ›</Button>
        </div>
        <p>表示期間：{yearMonthLabel(period[0])}〜{yearMonthLabel(period[5])}</p>
        <Button variant="secondary" disabled={query.isFetching || isCoolingDown} onClick={() => void reload()}>{query.isFetching ? '再読み込み中…' : '再読み込み'}</Button>
        {!canMovePrevious && <small>これより前の集計はありません。</small>}
      </section>

      {query.isRefetchError && <div className={styles.warning} role="alert"><p>更新できませんでした。前回取得した内容を表示しています。</p><Button variant="secondary" onClick={() => void reload()}>再試行</Button></div>}

      <section className={styles.trend} aria-labelledby="trend-heading">
        <div className={styles.sectionHeading}>
          <div><h2 id="trend-heading">月ごとの支出</h2><p>{mode === 'category' ? 'カテゴリ別の高さは明細合計です。計上額とは一致しない場合があります。' : '各支出の計上額を月ごとに合計しています。'}</p></div>
          <div className={styles.modeSwitch} role="group" aria-label="グラフの表示内容">
            <button type="button" aria-pressed={mode === 'category'} onClick={() => updateParams({ mode: 'category' })}>カテゴリ別（明細）</button>
            <button type="button" aria-pressed={mode === 'recorded'} onClick={() => updateParams({ mode: 'recorded' })}>計上額</button>
          </div>
        </div>
        {mode === 'category' && <ul className={styles.legend} aria-label="カテゴリの凡例">{dashboardCategories.map((category) => <li key={category}><span data-category={category} />{CATEGORY_LABELS[category]}</li>)}</ul>}
        <SummaryGraph
          mode={mode}
          period={period}
          summariesByMonth={summariesByMonth}
          onOpenMonth={rememberScrollPosition}
        />
        {summariesByMonth.has(nowMonth) && period.includes(nowMonth) && <p className={styles.currentMonthNote}>※ {yearMonthLabel(nowMonth)}は月途中の登録分です。前月全体との比較になります。</p>}
      </section>

      <section className={styles.breakdown} aria-labelledby="breakdown-heading">
        <div className={styles.sectionHeading}>
          <div><h2 id="breakdown-heading">カテゴリ別内訳</h2><p>明細ベース・参考</p></div>
          <label>対象月
            <select value={referenceMonth} onChange={(event) => updateParams({ reference: event.target.value })}>
              {period.map((month) => <option key={month} value={month}>{yearMonthLabel(month)}</option>)}
            </select>
          </label>
        </div>
        <p className={styles.help}>明細金額の合計です。月の計上額とは一致しない場合があります。</p>
        {!referenceSummary ? (
          <div className={styles.referenceEmpty}><p>この月の集計はありません。</p><Link to={`/months/${referenceMonth}`}>{yearMonthLabel(referenceMonth)}の明細を見る ›</Link></div>
        ) : (
          <>
            {referenceRows.length === 0 ? <p className={styles.referenceEmpty}>{referenceSummary.detail_count === 0 ? 'この月の明細はありません。' : 'この月の明細金額はすべて0円です。'}</p> : (
              <dl className={styles.categoryList}>{referenceRows.map((row) => <div key={row.category}><dt><span data-category={row.category} />{row.label}</dt><dd className="amount">{formatYen(row.amount)}</dd></div>)}</dl>
            )}
            <dl className={styles.summaryList}>
              <div><dt>明細合計</dt><dd className="amount">{formatYen(detailTotal(referenceSummary))}</dd></div>
              <div><dt>明細数</dt><dd>{referenceSummary.detail_count}件</dd></div>
              <div><dt>対象月の計上額</dt><dd className="amount">{formatYen(referenceSummary.total_recorded_amount)}</dd></div>
              <div><dt>前月からの変化</dt><dd className="amount">{previousReference ? signedYen(referenceSummary.total_recorded_amount - previousReference.total_recorded_amount) : '比較できません'}</dd></div>
            </dl>
            <Link className={styles.detailsLink} to={`/months/${referenceMonth}`} onClick={rememberScrollPosition}>{yearMonthLabel(referenceMonth)}の明細を見る ›</Link>
            <p className={styles.updatedAt}>集計更新：{formatUpdatedAt(referenceSummary.updated_at)}</p>
          </>
        )}
      </section>
    </div>
  )
}
