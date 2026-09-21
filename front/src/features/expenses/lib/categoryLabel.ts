// カテゴリの表示名。保存値は docs/ddd/ubiquitous-language.md の語彙と同じ
const CATEGORY_LABELS: Record<string, string> = {
  food: '食費',
  daily_goods: '日用品',
  medical: '医療',
  transport: '交通',
  utilities: '水道・光熱・通信',
  entertainment: '娯楽',
  social: '交際・会食',
  clothing: '衣類',
  education: '教育',
  other: 'その他',
  unknown: '分類不能',
}

export function categoryLabel(category: string): string {
  return CATEGORY_LABELS[category] ?? category
}
