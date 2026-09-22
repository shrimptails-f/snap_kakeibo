type ExpenseLocationState = {
  from?: string
  backLabel?: string
  detailId?: string
  requestId?: string
  scrollY?: number
  analysisCursorHistory?: string[]
}

export function expenseBackLink(search: string, state: ExpenseLocationState | null, yearMonth?: string) {
  const params = new URLSearchParams(search)
  const requestId = params.get('request')
  const isFromUpload = params.get('from') === 'upload'
  const to = state?.from ?? (isFromUpload
    ? `/upload${requestId ? `?request=${encodeURIComponent(requestId)}` : ''}`
    : yearMonth ? `/months/${yearMonth}` : '/analysis-requests')
  const label = state?.backLabel ?? (isFromUpload ? 'アップロードへ' : yearMonth ? `${yearMonth.replace('-', '年')}月の支出へ` : '解析履歴へ')
  const returnState = state?.detailId
    ? { restoreDetailId: state.detailId, scrollY: state.scrollY }
    : state?.requestId
      ? { restoreRequestId: state.requestId, scrollY: state.scrollY, analysisCursorHistory: state.analysisCursorHistory }
      : undefined
  return { to, label, state: returnState }
}

export type { ExpenseLocationState }
