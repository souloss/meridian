export function responseData<T>(response: unknown): T | undefined {
  if (!response || typeof response !== 'object' || !('status' in response) || !('data' in response)) {
    return undefined
  }
  const status = response.status
  if (typeof status !== 'number' || status < 200 || status >= 300) {
    return undefined
  }
  return response.data as T
}

export function errorMessage(error: unknown, fallback = '请求失败，请稍后重试。'): string {
  if (error && typeof error === 'object' && 'message' in error && typeof error.message === 'string') {
    return error.message
  }
  return fallback
}
