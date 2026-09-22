import { useEffect, useRef, useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { Link, useBlocker, useLocation } from 'react-router'
import { useFieldArray, useForm, useWatch } from 'react-hook-form'
import { getExpense } from '../api/expenses.api'
import { expenseQueryKey, useExpense } from '../hooks/useExpense'
import { useUpdateExpense } from '../hooks/useUpdateExpense'
import { categoryLabel } from '../lib/categoryLabel'
import { updateExpenseRequestSchema } from '../types/expense.schema'
import type { ExpenseDetail, GetExpenseResponse, UpdateExpenseRequest } from '../types/expense.types'
import { formatYen } from '@/shared/lib/formatYen'
import { Button } from '@/shared/ui/Button'
import { SourceBadge } from './SourceBadge'
import { ReceiptImage } from './ReceiptImage'
import styles from './ExpenseDetailContent.module.css'

const CATEGORIES = ['food', 'daily_goods', 'medical', 'transport', 'utilities', 'entertainment', 'social', 'clothing', 'education', 'other', 'unknown'] as const

type LocationState = { from?: string; backLabel?: string }
type Props = { expenseId: string }

function defaultValues(data: GetExpenseResponse): UpdateExpenseRequest {
  return { store_name: data.expense.store_name, purchase_date: data.expense.purchase_date, adjustment_amount: data.expense.adjustment_amount, details: data.details.map(({ detail_id, name, amount, quantity, category }) => ({ detail_id, name, amount, quantity, category: category as UpdateExpenseRequest['details'][number]['category'] })) }
}

function DetailList({ details }: { details: ExpenseDetail[] }) {
  const total = details.reduce((sum, detail) => sum + detail.amount, 0)
  return (
    <section className={styles.detailsSection} aria-labelledby="detail-heading">
      <h2 id="detail-heading">支出明細 <span>{details.length}件</span></h2>
      <ol className={styles.detailList}>
        {details.map((detail) => <li key={detail.detail_id}><div><strong>{detail.name}</strong><span>{categoryLabel(detail.category)} / 数量 {detail.quantity}</span><SourceBadge source={detail.source} isEdited={detail.is_edited} /></div><strong className="amount">{formatYen(detail.amount)}</strong></li>)}
      </ol>
      <div className={styles.detailTotal}><span>明細合計</span><strong className="amount">{formatYen(total)}</strong></div>
      <p className={styles.help}>明細合計と読取金額は、店舗の値引きや税などにより異なる場合があります。</p>
    </section>
  )
}

export function ExpenseDetailContent({ expenseId }: Props) {
  const { data } = useExpense(expenseId)
  const [isEditing, setIsEditing] = useState(false)
  const [notice, setNotice] = useState<string | null>(null)
  const [isRefreshFailed, setIsRefreshFailed] = useState(false)
  const [wantsCancel, setWantsCancel] = useState(false)
  const queryClient = useQueryClient()
  const mutation = useUpdateExpense(expenseId)
  const location = useLocation()
  const state = location.state as LocationState | null
  const cameFromUpload = new URLSearchParams(location.search).get('from') === 'upload'
  const backPath = state?.from ?? (cameFromUpload ? '/' : `/months/${data.expense.year_month}`)
  const backLabel = state?.backLabel ?? (cameFromUpload ? 'レシート取り込みへ' : `${data.expense.year_month.replace('-', '年')}月の支出へ`)
  const noticeRef = useRef<HTMLDivElement>(null)

  const form = useForm<UpdateExpenseRequest>({ resolver: zodResolver(updateExpenseRequestSchema), defaultValues: defaultValues(data), mode: 'onBlur' })
  const { handleSubmit } = form
  const fields = useFieldArray({ control: form.control, name: 'details' }).fields
  const adjustment = useWatch({ control: form.control, name: 'adjustment_amount' })
  const detailValues = useWatch({ control: form.control, name: 'details' })
  const recordedAmount = Number.isSafeInteger(adjustment) ? data.expense.read_amount + adjustment : null
  const detailTotal = detailValues.reduce((sum, detail) => sum + (Number.isFinite(detail.amount) ? detail.amount : 0), 0)
  const isDirty = form.formState.isDirty
  const blocker = useBlocker(isEditing && isDirty && !mutation.isPending)

  useEffect(() => {
    if (!isEditing || !isDirty) return
    const warn = (event: BeforeUnloadEvent) => { event.preventDefault() }
    window.addEventListener('beforeunload', warn)
    return () => window.removeEventListener('beforeunload', warn)
  }, [isEditing, isDirty])

  async function handleSave(values: UpdateExpenseRequest) {
    form.clearErrors('root.server')
    setIsRefreshFailed(false)
    try {
      await mutation.mutateAsync(values)
    } catch {
      form.setError('root.server', { message: '変更を保存できませんでした。入力内容は残っています。' })
      return
    }
    await queryClient.invalidateQueries({ queryKey: ['monthly-expenses'] })
    await queryClient.invalidateQueries({ queryKey: ['monthly-summaries'] })
    try {
      const fresh = await queryClient.fetchQuery({ queryKey: expenseQueryKey(expenseId), queryFn: ({ signal }) => getExpense(expenseId, signal), staleTime: 0 })
      form.reset(defaultValues(fresh)); setIsEditing(false); setNotice('変更を保存しました。')
      requestAnimationFrame(() => noticeRef.current?.focus())
    } catch {
      setIsRefreshFailed(true)
    }
  }

  async function reloadLatest() {
    setIsRefreshFailed(false)
    try { const fresh = await queryClient.fetchQuery({ queryKey: expenseQueryKey(expenseId), queryFn: ({ signal }) => getExpense(expenseId, signal), staleTime: 0 }); form.reset(defaultValues(fresh)); setIsEditing(false); setNotice('最新の内容を読み込みました。') }
    catch { setIsRefreshFailed(true) }
  }

  const discard = () => { form.reset(defaultValues(data)); setIsEditing(false); setWantsCancel(false); blocker.reset?.() }
  const confirmOpen = wantsCancel || blocker.state === 'blocked'

  return (
    <>
      <Link className={styles.back} to={backPath}>&lt; {backLabel}</Link>
      <h1>{isEditing ? '支出を編集' : '支出詳細'}</h1>
      {notice && <div className={styles.success} role="status" tabIndex={-1} ref={noticeRef}>{notice}</div>}
      {isRefreshFailed && <div className={styles.warning} role="alert"><p>保存は完了しましたが、最新の表示を取得できませんでした。</p><Button variant="secondary" onClick={reloadLatest}>最新の内容を読み込む</Button></div>}

      <div className={styles.layout}>
        <section className={styles.amountSummary} aria-label="支出金額">
          <div><span>{isEditing ? '保存後の計上額' : '計上額'}</span><strong className="amount">{recordedAmount === null ? '入力中' : formatYen(recordedAmount)}</strong></div>
          <p><span>読取金額 {formatYen(data.expense.read_amount)}</span><span>＋ 調整額 {recordedAmount === null ? '入力中' : formatYen(adjustment)}</span></p>
          {!isEditing && <SourceBadge source={data.expense.source} isEdited={data.expense.is_edited} />}
          {recordedAmount !== null && recordedAmount < 0 && <small>返金などにより、今月の支出を減らす金額です。</small>}
        </section>
        <aside className={styles.receipt}><h2>レシート画像</h2><ReceiptImage expenseId={expenseId} initialUrl={data.expense.image_url} /></aside>
        <div className={styles.content}>
          {!isEditing ? (
            <>
              <dl className={styles.metadata}><div><dt>店舗名</dt><dd>{data.expense.store_name}</dd></div><div><dt>購入日</dt><dd>{data.expense.purchase_date}</dd></div></dl>
              <DetailList details={data.details} />
              <Button className={styles.editButton} variant="primary" onClick={() => { setNotice(null); setIsEditing(true); requestAnimationFrame(() => document.getElementById('expense-form-heading')?.focus()) }}>編集する</Button>
            </>
          ) : (
            <>
              {/* react-hook-form の handleSubmit は submit event の時だけ ref を読む。React Compiler の静的判定をこの行だけ抑止する。 */}
              {/* oxlint-disable-next-line react/refs */}
              <form className={styles.form} onSubmit={handleSubmit(handleSave)} noValidate>
              <h2 id="expense-form-heading" tabIndex={-1}>編集内容</h2>
              <p className={isDirty ? styles.unsaved : styles.unchanged}>{isDirty ? '変更あり・未保存' : '変更はありません'}</p>
              {form.formState.errors.root?.server && <p className={styles.error} role="alert">{form.formState.errors.root.server.message}</p>}
              {Object.keys(form.formState.errors).some((key) => key !== 'root') && <div className={styles.errorSummary} role="alert"><strong>入力内容を確認してください。</strong></div>}
              <fieldset disabled={mutation.isPending}>
                <label>店舗名（必須）<input {...form.register('store_name')} aria-invalid={!!form.formState.errors.store_name} />{form.formState.errors.store_name && <span>{form.formState.errors.store_name.message}</span>}</label>
                <label>購入日（必須）<input type="date" {...form.register('purchase_date')} aria-invalid={!!form.formState.errors.purchase_date} />{form.formState.errors.purchase_date && <span>{form.formState.errors.purchase_date.message}</span>}</label>
                <label>調整額（必須）<small>減らす場合は「-500」、増やす場合は「500」。調整なしは0。</small><span className={styles.inputUnit}><input type="number" inputMode="numeric" {...form.register('adjustment_amount', { valueAsNumber: true })} aria-invalid={!!form.formState.errors.adjustment_amount} /><span>円</span></span>{form.formState.errors.adjustment_amount && <span>{form.formState.errors.adjustment_amount.message}</span>}</label>
                <section className={styles.editDetails}><h2>支出明細 <span>{fields.length}件</span></h2>
                  {fields.map((field, index) => { const error = form.formState.errors.details?.[index]; const saved = data.details[index]; return <fieldset className={styles.detailFields} key={field.id}><legend>{index + 1}. <SourceBadge source={saved?.source} isEdited={saved?.is_edited ?? false} /></legend><input type="hidden" {...form.register(`details.${index}.detail_id`)} /><label>商品名（必須）<input {...form.register(`details.${index}.name`)} aria-invalid={!!error?.name} />{error?.name && <span>{error.name.message}</span>}</label><div className={styles.fieldRow}><label>明細金額（必須）<span className={styles.inputUnit}><input type="number" inputMode="numeric" {...form.register(`details.${index}.amount`, { valueAsNumber: true })} aria-invalid={!!error?.amount} /><span>円</span></span>{error?.amount && <span>{error.amount.message}</span>}</label><label>数量（必須）<input type="number" inputMode="numeric" {...form.register(`details.${index}.quantity`, { valueAsNumber: true })} aria-invalid={!!error?.quantity} />{error?.quantity && <span>{error.quantity.message}</span>}</label></div><label>カテゴリ（必須）<select {...form.register(`details.${index}.category`)}>{CATEGORIES.map((category) => <option key={category} value={category}>{categoryLabel(category)}</option>)}</select></label><SourceBadge label="カテゴリ" source={saved?.category_source} isEdited={saved?.category_source === 'USER'} /></fieldset> })}
                </section>
              </fieldset>
              <div className={styles.detailTotal}><span>明細合計</span><strong className="amount">{formatYen(detailTotal)}</strong></div>
              <p className={styles.help}>明細の修正だけでは計上額は変わりません。計上額も直す場合は、調整額を変更してください。</p>
              <div className={styles.saveBar}><span>{isDirty ? '未保存' : '変更なし'} / 計上額 {recordedAmount === null ? '入力中' : formatYen(recordedAmount)}</span><div><Button variant="secondary" disabled={mutation.isPending} onClick={() => isDirty ? setWantsCancel(true) : discard()}>キャンセル</Button><Button variant="primary" type="submit" disabled={!isDirty || mutation.isPending}>{mutation.isPending ? '保存中…' : '変更を保存'}</Button></div></div>
              </form>
            </>
          )}
        </div>
      </div>

      {confirmOpen && <div className={styles.dialogBackdrop}><div className={styles.dialog} role="dialog" aria-modal="true" aria-labelledby="discard-title"><h2 id="discard-title">変更を破棄しますか？</h2><p>保存していない変更は失われます。</p><div><Button variant="secondary" autoFocus onClick={() => { setWantsCancel(false); blocker.reset?.() }}>編集を続ける</Button><Button variant="danger" onClick={() => { if (blocker.state === 'blocked') blocker.proceed(); else discard() }}>変更を破棄する</Button></div></div></div>}
    </>
  )
}
