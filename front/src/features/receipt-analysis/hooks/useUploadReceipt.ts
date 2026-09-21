import { useMutation, useQueryClient } from '@tanstack/react-query'
import { createUpload, uploadToPresignedUrl } from '../api/uploads.api'
import { analysisRequestsQueryKey } from './useAnalysisRequests'

type UploadReceiptInput = {
  file: File
  yearMonth: string
}

// レシート画像を 1 枚アップロードする。完了したら一覧を再取得して UPLOADING の行を反映する
export function useUploadReceipt() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async ({ file }: UploadReceiptInput) => {
      const contentType = file.type || 'image/jpeg'
      const created = await createUpload({ file_name: file.name, content_type: contentType })
      await uploadToPresignedUrl(created.put_url, file, contentType)
      return created
    },
    onSuccess: (_created, { yearMonth }) => {
      return queryClient.invalidateQueries({ queryKey: analysisRequestsQueryKey(yearMonth) })
    },
  })
}
