import { authService } from '@/services/auth'

const API_BASE_URL = import.meta.env.DEV ? '' : (import.meta.env.VITE_API_BASE_URL || '')

type UnauthorizedCallback = () => void | Promise<void>
let onUnauthorizedCallback: UnauthorizedCallback | null = null

export function setOnUnauthorized(callback: UnauthorizedCallback) {
  onUnauthorizedCallback = callback
}

// Ported from com.sixt.web.managed-agents/src/services/http.ts: refresh the
// token proactively, attach it as a Bearer header, and redirect to login on
// a 401 instead of leaving the caller in a stuck "loading" state.
export async function authedFetch(path: string, init: RequestInit = {}): Promise<Response> {
  const tokenValid = await authService.ensureTokenValid(30)
  if (!tokenValid && authService.isAuthenticated()) {
    console.warn('[http] Token refresh failed - session expired')
    if (onUnauthorizedCallback) await onUnauthorizedCallback()
    return new Promise<Response>(() => {})
  }

  const headers = new Headers(init.headers)
  const token = authService.getToken()
  if (token) headers.set('Authorization', `Bearer ${token}`)

  const response = await fetch(`${API_BASE_URL}${path}`, { ...init, headers })

  if (response.status === 401) {
    console.warn('[http] Received 401 Unauthorized - redirecting to login')
    if (onUnauthorizedCallback) await onUnauthorizedCallback()
    return new Promise<Response>(() => {})
  }

  return response
}
