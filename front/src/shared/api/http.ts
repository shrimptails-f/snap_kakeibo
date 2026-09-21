import { authRefreshEndpoint, refreshAuthSession } from '@/shared/auth/auth.api'
import { getAuthorizationHeaderValue, hasAuthToken } from '@/shared/auth/token'
import { apiBaseUrl } from '@/shared/config/env'
import { Client, type RequestOptions } from './client'

type BodyLessRequestOptions = Omit<RequestOptions<never>, 'body'>

// feature の api 関数はこの client(または下の http)だけを経由してバックエンドを呼ぶ
export const apiClient = new Client({
  baseUrl: apiBaseUrl,
  defaultCredentials: 'include',
  auth: {
    refreshEndpoint: authRefreshEndpoint,
    getAuthorizationHeaderValue,
    hasAuthToken,
    refreshAuthSession,
  },
})

export function get<TResponse>(endpoint: string, options: BodyLessRequestOptions = {}): Promise<TResponse> {
  return apiClient.request<TResponse>('GET', endpoint, options)
}

export function post<TResponse, TBody = unknown>(
  endpoint: string,
  options: RequestOptions<TBody> = {},
): Promise<TResponse> {
  return apiClient.request<TResponse, TBody>('POST', endpoint, options)
}

export function put<TResponse, TBody = unknown>(
  endpoint: string,
  options: RequestOptions<TBody> = {},
): Promise<TResponse> {
  return apiClient.request<TResponse, TBody>('PUT', endpoint, options)
}

export function patch<TResponse, TBody = unknown>(
  endpoint: string,
  options: RequestOptions<TBody> = {},
): Promise<TResponse> {
  return apiClient.request<TResponse, TBody>('PATCH', endpoint, options)
}

export function deleteRequest<TResponse>(
  endpoint: string,
  options: BodyLessRequestOptions = {},
): Promise<TResponse> {
  return apiClient.request<TResponse>('DELETE', endpoint, options)
}

export const http = {
  get,
  post,
  put,
  patch,
  delete: deleteRequest,
}
