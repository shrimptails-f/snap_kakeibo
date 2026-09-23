import { useEffect, useMemo, useRef, type CSSProperties } from 'react'
import { Link, useSearchParams } from 'react-router'
import { formatYen } from '@/shared/lib/formatYen'
import { useReloadCooldown } from '@/shared/hooks/useReloadCooldown'
import { ReloadButton, type ReloadControl } from '@/shared/ui/ReloadButton'
import { SpinnerBlock } from '@/shared/ui/Spinner'
import { useMonthlySummaries } from '../hooks/useMonthlySummaries'
import {
  CATEGORY_LABELS,
  categoryRows,
  currentYearMonth,
  detailTotal,
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
  referenceMonth,
  onSelectReferenceMonth,
}: {
  mode: ChartMode
  period: string[]
  summariesByMonth: Map<string, MonthlySummary>
  onOpenMonth: () => void
  referenceMonth: string
  onSelectReferenceMonth: (month: string) => void
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
                  <button type="button" className={styles.categoryButton} aria-label={`${yearMonthLabel(month)}のカテゴリ別内訳を表示`} aria-pressed={referenceMonth === month} onClick={() => onSelectReferenceMonth(month)}>
                  {segmentPositions(summary, scale.minimum, scale.range).map((segment) => (
                    <span
                      key={segment.category}
                      className={styles.segment}
                      data-category={segment.category}
                      style={valueStyle(segment.start, segment.size)}
                      aria-hidden="true"
                    />
                  ))
                  }</button>
                ) : (
                  <button
                    type="button"
                    className={styles.recordedButton}
                    onClick={() => onSelectReferenceMonth(month)}
                    aria-label={`${yearMonthLabel(month)}のカテゴリ別内訳を表示（計上額${formatYen(summary.total_recorded_amount)}）`}
                    aria-pressed={referenceMonth === month}
                  >
                    <span
                      className={styles.recordedBar}
                      style={valueStyle(
                        ((Math.min(0, summary.total_recorded_amount) - scale.minimum) / scale.range) * 100,
                        (Math.abs(summary.total_recorded_amount) / scale.range) * 100,
                      )}
                    />
                  </button>
                )}
              </div>
              <Link className={styles.monthLink} to={`/months/${month}`} onClick={onOpenMonth}>{shortMonthLabel(month, previousMonth)}</Link>
              <strong className={`amount ${styles.graphValue}`}>{visibleAmount === null ? '集計なし' : formatYen(visibleAmount)}</strong>
              {summary && <small>{previousMonthChange(summary, summariesByMonth)}</small>}
            </li>
          )
        })}
      </ol>
    </div>
  )
}

function InitialError({ reload }: { reload: ReloadControl }) {
  return (
    <section className={styles.error} role="alert">
      <h2>月ごとの支出を取得できませんでした</h2>
      <p>通信状態を確認して、もう一度お試しください。</p>
      <ReloadButton reload={reload} idleLabel="再試行" className={styles.dashboardReload} />
    </section>
  )
}

export function DashboardPage() {
  const query = useMonthlySummaries()
  const [searchParams, setSearchParams] = useSearchParams()
  const cooldown = useReloadCooldown()
  const didRestoreScroll = useRef(false)
  const scrollStorageKey = `dashboard-scroll:${searchParams.toString()}`
  const nowMonth = currentYearMonth()
  const summaries = useMemo(
    () => [...(query.data?.monthly_summaries ?? [])].sort((left, right) => right.year_month.localeCompare(left.year_month)),
    [query.data],
  )
  const summariesByMonth = useMemo(() => new Map(summaries.map((summary) => [summary.year_month, summary])), [summaries])

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
  const selectableEndMonths = monthsBetween(earliestSelectableMonth, latestSelectableMonth)
  const selectableStartMonths = monthsBetween(shiftYearMonth(earliestSelectableMonth, -5), shiftYearMonth(latestSelectableMonth, -5))

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

  function selectStartMonth(nextStartMonth: string) {
    selectEndMonth(shiftYearMonth(nextStartMonth, 5))
  }

  function rememberScrollPosition() {
    window.sessionStorage.setItem(scrollStorageKey, String(window.scrollY))
  }

  async function reload() {
    if (!cooldown.startCooldown()) return
    await query.refetch()
  }

  const reloadControl: ReloadControl = {
    reload: () => void reload(),
    isDisabled: query.isFetching || cooldown.isCoolingDown,
    isFetching: query.isFetching,
    isCoolingDown: cooldown.isCoolingDown,
    cooldownRemainingMs: cooldown.cooldownRemainingMs,
  }

  if (query.isError && !query.data) {
    return (
      <div className={styles.page}>
        <header className={styles.pageHeader}><div><h1>ダッシュボード</h1><p>月ごとの支出を振り返る</p></div></header>
        <InitialError reload={reloadControl} />
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
          <ReloadButton reload={reloadControl} className={styles.dashboardReload} />
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
        <Link className={styles.uploadLink} to="/upload">レシートを取り込む</Link>
      </header>

      <section className={styles.periodControls} aria-labelledby="period-heading">
        <h2 id="period-heading" className={styles.visuallyHidden}>表示期間</h2>
        <label>表示開始月
          <select value={period[0]} onChange={(event) => selectStartMonth(event.target.value)}>
            {selectableStartMonths.map((month) => <option key={month} value={month}>{yearMonthLabel(month)}</option>)}
          </select>
        </label>
        <label>表示終了月
          <select value={endMonth} onChange={(event) => selectEndMonth(event.target.value)}>
            {selectableEndMonths.map((month) => <option key={month} value={month}>{yearMonthLabel(month)}{month > nowMonth ? '（未来月）' : ''}</option>)}
          </select>
        </label>
        <ReloadButton reload={reloadControl} className={styles.dashboardReload} />
      </section>

      {query.isRefetchError && <div className={styles.warning} role="alert"><p>更新できませんでした。前回取得した内容を表示しています。</p><ReloadButton reload={reloadControl} idleLabel="再試行" className={styles.dashboardReload} /></div>}

      <section className={styles.trend} aria-labelledby="trend-heading">
        <div className={styles.sectionHeading}>
          <div><h2 id="trend-heading">月ごとの支出</h2><p>{mode === 'category' ? 'カテゴリ別の高さは確定した税込み明細額と未確定の印字額の合計です。計上額とは一致しない場合があります。' : '各支出の計上額を月ごとに合計しています。'}</p></div>
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
          referenceMonth={referenceMonth}
          onSelectReferenceMonth={(month) => updateParams({ reference: month })}
        />
      </section>

      <section className={styles.breakdown} aria-labelledby="breakdown-heading">
        <div className={styles.sectionHeading}>
          <div><h2 id="breakdown-heading">カテゴリ別内訳</h2><p>{yearMonthLabel(referenceMonth)}</p></div>
          <Link className={styles.detailsLink} to={`/months/${referenceMonth}`} onClick={rememberScrollPosition}>{yearMonthLabel(referenceMonth)}の明細を見る ›</Link>
        </div>
        <p className={styles.help}>確定した税込み明細額と未確定の印字額を集計します。店舗の値引き、利用者の調整、読取漏れにより月の計上額と一致しない場合があります。</p>
        {!referenceSummary ? (
          <div className={styles.referenceEmpty}><p>この月の集計はありません。</p></div>
        ) : (
          <>
            {referenceSummary.detail_count > (referenceSummary.confirmed_detail_count ?? 0) && <p className={styles.help}>税込み額未確定の明細 {referenceSummary.detail_count - (referenceSummary.confirmed_detail_count ?? 0)}件は印字額で含めています。</p>}
            {referenceRows.length === 0 ? <p className={styles.referenceEmpty}>{referenceSummary.detail_count === 0 ? 'この月の明細はありません。' : 'この月の明細金額はすべて0円です。'}</p> : (
              <dl className={styles.categoryList}>{referenceRows.map((row) => <div key={row.category}><dt><span data-category={row.category} />{row.label}</dt><dd className="amount">{formatYen(row.amount)}</dd></div>)}</dl>
            )}
            <dl className={styles.summaryList}>
              <div><dt>明細合計</dt><dd className="amount">{formatYen(detailTotal(referenceSummary))}</dd></div>
              <div><dt>明細数</dt><dd>{referenceSummary.detail_count}件</dd></div>
              <div><dt>対象月の計上額</dt><dd className="amount">{formatYen(referenceSummary.total_recorded_amount)}</dd></div>
              <div><dt>前月からの変化</dt><dd className="amount">{previousReference ? signedYen(referenceSummary.total_recorded_amount - previousReference.total_recorded_amount) : '比較できません'}</dd></div>
            </dl>
          </>
        )}
      </section>
    </div>
  )
}
