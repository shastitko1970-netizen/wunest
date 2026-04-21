/**
 * Thin fetch wrapper. All API calls go through here so we can add retries,
 * toast-on-error, CSRF token handling, etc. in one place later.
 */

export class ApiError extends Error {
  constructor(public readonly status: number, message: string) {
    super(message)
  }
}

export async function apiFetch<T>(path: string, init: RequestInit = {}): Promise<T> {
  const res = await fetch(path, {
    credentials: 'include', // send .wusphere.ru cookies
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...init.headers,
    },
  })

  if (!res.ok) {
    const body = await res.text().catch(() => '')
    throw new ApiError(res.status, body || res.statusText)
  }

  // 204 no content / empty body
  const text = await res.text()
  return text ? (JSON.parse(text) as T) : (undefined as T)
}
