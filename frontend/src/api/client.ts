/**
 * Thin fetch wrapper. All API calls go through here so we can add retries,
 * toast-on-error, CSRF token handling, etc. in one place later.
 */

/**
 * Structured payload for HTTP 402 responses raised by the slot-limit
 * enforcement middleware (M54.2). Server emits this exact shape so the
 * SPA can render an "upgrade?" dialog without parsing the textual error.
 */
export interface LimitReachedDetail {
  kind: 'limit_reached'
  resource: 'character' | 'lorebook' | 'persona' | 'preset'
  current: number
  max: number
}

/**
 * 403 envelope for BYOK keys pointing at provider hosts on our
 * blocklist (e.g. EllyAI). The server refuses to store the key and
 * returns this so the SPA can render a banner explaining why instead
 * of a generic "forbidden" toast.
 */
export interface BlockedProviderDetail {
  kind: 'blocked_provider'
  provider_host: string
  provider_label: string
  message: string
}

/** Discriminated union of all structured error envelopes. New error
 *  shapes plug in by extending this union — `apiFetch` automatically
 *  attaches the detail object to ApiError when it sees a known kind. */
export type ApiErrorDetail = LimitReachedDetail | BlockedProviderDetail

export class ApiError extends Error {
  /**
   * Decoded JSON envelope for structured errors. Set when the server
   * returned a body matching one of the kinds in ApiErrorDetail; plain
   * text errors leave this undefined.
   */
  public readonly detail?: ApiErrorDetail

  constructor(public readonly status: number, message: string, detail?: ApiErrorDetail) {
    super(message)
    this.detail = detail
  }
}

/** Type guard for the structured limit-reached error. */
export function isLimitReached(err: unknown): err is ApiError & { detail: LimitReachedDetail } {
  return err instanceof ApiError && err.status === 402 && err.detail?.kind === 'limit_reached'
}

/** Type guard for the structured blocked-provider error. */
export function isBlockedProvider(err: unknown): err is ApiError & { detail: BlockedProviderDetail } {
  return err instanceof ApiError && err.status === 403 && err.detail?.kind === 'blocked_provider'
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
    // Structured-error decode. Two known kinds today:
    //   - 402 + kind=limit_reached       (slot caps, M54.2)
    //   - 403 + kind=blocked_provider    (BYOK blocklist)
    // Falls through to a plain-text ApiError if the body isn't JSON
    // or doesn't carry one of the expected envelopes.
    if ((res.status === 402 || res.status === 403) && body) {
      try {
        const parsed = JSON.parse(body)
        if (parsed && (parsed.kind === 'limit_reached' || parsed.kind === 'blocked_provider')) {
          throw new ApiError(res.status, body, parsed as ApiErrorDetail)
        }
      } catch (e) {
        if (e instanceof ApiError) throw e
        // JSON.parse failed — fall through to plain ApiError below.
      }
    }
    throw new ApiError(res.status, body || res.statusText)
  }

  // 204 no content / empty body
  const text = await res.text()
  return text ? (JSON.parse(text) as T) : (undefined as T)
}
