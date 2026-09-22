import { z } from 'zod'

// ログインフォームの入力。文言は利用者向け(design_guidelines.md §6)
export const loginFormSchema = z.object({
  email: z.email({ error: 'メールアドレスの形式で入力してください。' }),
  password: z.string().min(1, { error: 'パスワードを入力してください。' }),
})

export type LoginFormValues = z.infer<typeof loginFormSchema>
