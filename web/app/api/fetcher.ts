export class MeridianApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    readonly requestId: string | null,
    readonly details: unknown
  ) {
    super(code)
  }
}

let csrfToken: string | undefined

export function setCsrfToken(token: string | undefined): void {
  csrfToken = token
}

export async function meridianFetch<T>(url: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers)
  headers.set('Accept', 'application/json')
  const method = options.method?.toUpperCase() ?? 'GET'
  if (csrfToken && !['GET', 'HEAD', 'OPTIONS'].includes(method)) {
    headers.set('X-CSRF-Token', csrfToken)
  }

  const response = await fetch(url, {
    ...options,
    headers,
    credentials: 'same-origin'
  })
  if (!response.ok) {
    const payload = await response.json().catch(() => ({})) as Record<string, unknown>
    throw new MeridianApiError(
      response.status,
      typeof payload.code === 'string' ? payload.code : 'request_failed',
      response.headers.get('X-Request-Id'),
      payload.details
    )
  }

  let data: unknown
  if (response.status !== 204) {
    const contentType = response.headers.get('Content-Type') ?? ''
    if (contentType.includes('json')) {
      data = await response.json()
    } else if (contentType.startsWith('text/') || contentType.includes('yaml')) {
      data = await response.text()
    } else {
      data = await response.blob()
    }
  }

  return { data, status: response.status, headers: response.headers } as T
}
