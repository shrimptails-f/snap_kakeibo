import { useEffect, useRef, useState } from 'react'
import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { Link, useBlocker, useLocation } from 'react-router'
import { useFieldArray, useForm, useWatch } from 'react-hook-form'
import { getExpense } from '../api/expenses.api'
import { expenseQueryKey, useExpense } from '../hooks/useExpense'
import { useUpdateExpense } from '../hooks/useUpdateExpense'
import { categoryLabel } from '../lib/categoryLabel'
import { expenseBackLink, type ExpenseLocationState } from '../lib/expenseBackLink'
import { updateExpenseRequestSchema } from '../types/expense.schema'
import type { ExpenseDetail, GetExpenseResponse, UpdateExpenseRequest } from '../types/expense.types'
import { formatYen } from '@/shared/lib/formatYen'
import { Button } from '@/shared/ui/Button'
import { SourceBadge } from './SourceBadge'
import { ReceiptImage } from './ReceiptImage'
import styles from './ExpenseDetailContent.module.css'

const CATEGORIES = ['food', 'daily_goods', 'medical', 'transport', 'utilities', 'entertainment', 'social', 'clothing', 'education', 'other', 'unknown'] as const

type Props = { expenseId: string }

function defaultValues(data: GetExpenseResponse): UpdateExpenseRequest {
  return { store_name: data.expense.store_name, purchase_date: data.expense.purchase_date, adjustment_amount: data.expense.adjustment_amount, details: data.details.map(({ detail_id, name, amount, quantity, category, tax_mode, tax_rate, tax_included_amount }) => ({ detail_id, name, amount, quantity, category: category as UpdateExpenseRequest['details'][number]['category'], tax_mode: tax_included_amount !== undefined && (tax_rate === 8 || tax_rate === 10) && (tax_mode === 'included' || tax_mode === 'external') ? tax_mode : 'unknown', tax_rate: tax_included_amount !== undefined && (tax_rate === 8 || tax_rate === 10) ? tax_rate : 0, tax_included_amount })) }
}

function DetailList({ details }: { details: ExpenseDetail[] }) {
  const total = details.reduce((sum, detail) => sum + (detail.tax_included_amount ?? detail.amount), 0)
  const unconfirmed = details.filter((detail) => detail.tax_included_amount === undefined).length
  return (
    <section className={styles.detailsSection} aria-labelledby="detail-heading">
      <h2 id="detail-heading">支出明細 <span>{details.length}件</span></h2>
      <ol className={styles.detailList}>
        {details.map((detail) => <li key={detail.detail_id}><div><strong>{detail.name}</strong><span>{categoryLabel(detail.category)} / 数量 {detail.quantity}</span><span>{detail.tax_included_amount === undefined ? '印字額・税込み未確定' : `税込み明細額（印字額 ${formatYen(detail.amount)}）`}{detail.tax_rate !== undefined ? ` / 税率 ${detail.tax_rate}%` : ''}{detail.tax_mode === 'external' && detail.tax_included_amount !== undefined ? ` / 配分税額 ${formatYen(detail.tax_included_amount - detail.amount)}` : ''}</span><SourceBadge source={detail.source} isEdited={detail.is_edited} /></div><strong className="amount">{formatYen(detail.tax_included_amount ?? detail.amount)}</strong></li>)}
      </ol>
      <div className={styles.detailTotal}><span>明細合計</span><strong className="amount">{formatYen(total)}</strong></div>
      <p className={styles.help}>{unconfirmed > 0 ? `税込み額未確定の明細 ${unconfirmed}件は印字額で含めています。` : ''}明細合計と最終支払合計は、店舗の値引きや読取漏れなどにより異なる場合があります。</p>
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
  const back = expenseBackLink(location.search, location.state as ExpenseLocationState | null, data.expense.year_month)
  const noticeRef = useRef<HTMLDivElement>(null)

  const form = useForm<UpdateExpenseRequest>({ resolver: zodResolver(updateExpenseRequestSchema), defaultValues: defaultValues(data), mode: 'onBlur' })
  const { handleSubmit } = form
  const { fields, append, remove } = useFieldArray({ control: form.control, name: 'details' })
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
      const initial = defaultValues(data)
      await mutation.mutateAsync({ ...values, details: values.details.map((detail) => {
        const previous = initial.details.find((row) => row.detail_id === detail.detail_id)
        const taxChanged = detail.tax_mode !== (previous?.tax_mode ?? 'unknown') || detail.tax_rate !== (previous?.tax_rate ?? 0) || detail.tax_included_amount !== previous?.tax_included_amount
        return { ...detail, tax_confirmed: taxChanged ? detail.tax_mode !== 'unknown' : undefined, tax_included_amount: detail.tax_mode === 'included' ? detail.amount : detail.tax_included_amount }
      }) })
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
      <Link className={styles.back} to={back.to} state={back.state}>&lt; {back.label}</Link>
      <h1>{isEditing ? '支出を編集' : '支出詳細'}</h1>
      {notice && <div className={styles.success} role="status" tabIndex={-1} ref={noticeRef}>{notice}</div>}
      {isRefreshFailed && <div className={styles.warning} role="alert"><p>保存は完了しましたが、最新の表示を取得できませんでした。</p><Button variant="secondary" onClick={reloadLatest}>最新の内容を読み込む</Button></div>}

      <div className={styles.layout}>
        <section className={styles.amountSummary} aria-label="支出金額">
          <div><span>{isEditing ? '保存後の計上額' : '計上額'}</span><strong className="amount">{recordedAmount === null ? '入力中' : formatYen(recordedAmount)}</strong></div>
          <p><span>最終支払合計（読取金額） {formatYen(data.expense.read_amount)}</span><span>＋ 利用者調整額 {recordedAmount === null ? '入力中' : formatYen(adjustment)}</span></p>
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
                <label>店舗名（必須）<input id="expense-store-name" className="field-control" {...form.register('store_name')} aria-invalid={!!form.formState.errors.store_name} aria-describedby={form.formState.errors.store_name ? 'expense-store-name-error' : undefined} />{form.formState.errors.store_name && <span id="expense-store-name-error">{form.formState.errors.store_name.message}</span>}</label>
                <label>購入日（必須）<input id="expense-purchase-date" className="field-control" type="date" {...form.register('purchase_date')} aria-invalid={!!form.formState.errors.purchase_date} aria-describedby={form.formState.errors.purchase_date ? 'expense-purchase-date-error' : undefined} />{form.formState.errors.purchase_date && <span id="expense-purchase-date-error">{form.formState.errors.purchase_date.message}</span>}</label>
                <label>調整額（必須）<small>減らす場合は「-500」、増やす場合は「500」。調整なしは0。</small><span className={styles.inputUnit}><input id="expense-adjustment" className="field-control" type="number" inputMode="numeric" {...form.register('adjustment_amount', { valueAsNumber: true })} aria-invalid={!!form.formState.errors.adjustment_amount} aria-describedby={form.formState.errors.adjustment_amount ? 'expense-adjustment-error' : undefined} /><span>円</span></span>{form.formState.errors.adjustment_amount && <span id="expense-adjustment-error">{form.formState.errors.adjustment_amount.message}</span>}</label>
                <section className={styles.editDetails}><h2>支出明細 <span>{fields.length}件</span></h2>
                  {fields.map((field, index) => {
                    const error = form.formState.errors.details?.[index]
                    const saved = data.details.find((detail) => detail.detail_id === field.detail_id)
                    const prefix = `expense-detail-${index}`
                    return <fieldset className={styles.detailFields} key={field.id}>
                      <legend>{index + 1}. {saved ? <SourceBadge source={saved.source} isEdited={saved.is_edited} /> : '追加予定'}</legend>
                      {saved && <input type="hidden" {...form.register(`details.${index}.detail_id`)} />}
                      <label>商品名（必須）<input id={`${prefix}-name`} className="field-control" {...form.register(`details.${index}.name`)} aria-invalid={!!error?.name} aria-describedby={error?.name ? `${prefix}-name-error` : undefined} />{error?.name && <span id={`${prefix}-name-error`}>{error.name.message}</span>}</label>
                      <div className={styles.fieldRow}>
                        <label>明細金額（必須）<span className={styles.inputUnit}><input id={`${prefix}-amount`} className="field-control" type="number" inputMode="numeric" {...form.register(`details.${index}.amount`, { valueAsNumber: true, onChange: () => { if (form.getValues(`details.${index}.tax_mode`) !== 'unknown') { form.setValue(`details.${index}.tax_mode`, 'unknown', { shouldDirty: true }); form.setValue(`details.${index}.tax_rate`, 0, { shouldDirty: true }); form.setValue(`details.${index}.tax_included_amount`, undefined, { shouldDirty: true }) } } })} aria-invalid={!!error?.amount} aria-describedby={error?.amount ? `${prefix}-amount-error` : undefined} /><span>円</span></span>{error?.amount && <span id={`${prefix}-amount-error`}>{error.amount.message}</span>}</label>
                        <label>数量（必須）<input id={`${prefix}-quantity`} className="field-control" type="number" inputMode="numeric" {...form.register(`details.${index}.quantity`, { valueAsNumber: true })} aria-invalid={!!error?.quantity} aria-describedby={error?.quantity ? `${prefix}-quantity-error` : undefined} />{error?.quantity && <span id={`${prefix}-quantity-error`}>{error.quantity.message}</span>}</label>
                      </div>
                      <label>カテゴリ（必須）<select id={`${prefix}-category`} className="field-control" {...form.register(`details.${index}.category`)} aria-invalid={!!error?.category} aria-describedby={error?.category ? `${prefix}-category-error` : undefined}>{CATEGORIES.map((category) => <option key={category} value={category}>{categoryLabel(category)}</option>)}</select>{error?.category && <span id={`${prefix}-category-error`}>{error.category.message}</span>}</label>
                      {saved && <SourceBadge label="カテゴリ" source={saved.category_source} isEdited={saved.category_source === 'USER'} />}
                      <p className={styles.help}>レシートで確認した税区分と税率を指定します。外税は税込み明細額も確認して入力してください。</p>
                      <div className={styles.fieldRow}>
                        <label>税区分<select className="field-control" {...form.register(`details.${index}.tax_mode`, { onChange: (event) => { if (event.target.value === 'external') form.setValue(`details.${index}.tax_included_amount`, undefined, { shouldDirty: true }) } })}><option value="unknown">未確定</option><option value="included">内税</option><option value="external">外税</option></select></label>
                        {detailValues[index]?.tax_mode !== 'unknown' && <label>税率<select className="field-control" {...form.register(`details.${index}.tax_rate`, { valueAsNumber: true, onChange: () => { if (form.getValues(`details.${index}.tax_mode`) === 'external') form.setValue(`details.${index}.tax_included_amount`, undefined, { shouldDirty: true }) } })} aria-invalid={!!error?.tax_rate}><option value="0">選択してください</option><option value="8">8%</option><option value="10">10%</option></select>{error?.tax_rate && <span>{error.tax_rate.message}</span>}</label>}
                      </div>
                      {detailValues[index]?.tax_mode === 'external' && <label>税込み明細額（必須）<span className={styles.inputUnit}><input className="field-control" type="number" inputMode="numeric" {...form.register(`details.${index}.tax_included_amount`, { setValueAs: (value: string) => value === '' ? undefined : Number(value) })} aria-invalid={!!error?.tax_included_amount} /><span>円</span></span>{error?.tax_included_amount && <span>{error.tax_included_amount.message}</span>}</label>}
                      <Button variant="secondary" disabled={fields.length <= 1} onClick={() => remove(index)} aria-label={`${index + 1}件目の明細を削除`}>この明細を削除</Button>
                    </fieldset>
                  })}
                  <Button variant="secondary" disabled={fields.length >= 50} onClick={() => append({ detail_id: undefined, name: '', amount: 0, quantity: 1, category: 'unknown', tax_mode: 'unknown', tax_rate: 0 })}>明細を追加</Button>
                </section>
              </fieldset>
              <div className={styles.detailTotal}><span>入力中の印字額合計</span><strong className="amount">{formatYen(detailTotal)}</strong></div>
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
