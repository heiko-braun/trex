import Keycloak from 'keycloak-js'
import type { AuthConfig, User } from '@/types'

const API_BASE_URL = import.meta.env.DEV ? '' : (import.meta.env.VITE_API_BASE_URL || '')

const baseUrl = import.meta.env.VITE_OIDC_REDIRECT_BASE_URL || window.location.origin
const redirectUri = `${baseUrl}/callback`
const postLogoutRedirectUri = baseUrl

type TokenExpiredCallback = () => void
type TokenRefreshedCallback = () => void

// Ported (trimmed) from com.sixt.web.managed-agents/src/services/auth.ts —
// same Keycloak PKCE flow and /api/v1/auth/config bootstrap, minus
// admin-role checks this app doesn't need yet (see specs/workflow-server-ui.md).
class AuthService {
  private keycloak: Keycloak | null = null
  private onTokenExpiredCallbacks: TokenExpiredCallback[] = []
  private onTokenRefreshedCallbacks: TokenRefreshedCallback[] = []
  private authConfig: AuthConfig | null = null
  private configPromise: Promise<AuthConfig> | null = null

  async fetchConfig(): Promise<AuthConfig> {
    if (this.authConfig) return this.authConfig
    if (this.configPromise) return this.configPromise

    this.configPromise = (async () => {
      const response = await fetch(`${API_BASE_URL}/api/v1/auth/config`)
      if (!response.ok) {
        throw new Error(`Failed to fetch auth config: ${response.status}`)
      }
      const config = (await response.json()) as AuthConfig
      this.authConfig = config
      return config
    })()

    return this.configPromise
  }

  async initialize(): Promise<boolean> {
    const config = await this.fetchConfig()

    const base = config.keycloak_url.replace(/\/+$/, '')
    const url = base.endsWith('/auth') ? base : `${base}/auth`

    this.keycloak = new Keycloak({
      url,
      realm: config.keycloak_realm,
      clientId: config.keycloak_client_id,
    })

    this.setupEventHandlers()

    try {
      return await this.keycloak.init({
        onLoad: 'check-sso',
        checkLoginIframe: false,
        pkceMethod: 'S256',
        redirectUri,
      })
    } catch (error) {
      console.error('[AuthService] Failed to initialize Keycloak:', error)
      return false
    }
  }

  private setupEventHandlers() {
    if (!this.keycloak) return

    this.keycloak.onTokenExpired = () => {
      this.keycloak!.updateToken(30)
        .then((refreshed) => {
          if (refreshed) this.onTokenRefreshedCallbacks.forEach((cb) => cb())
        })
        .catch(() => this.onTokenExpiredCallbacks.forEach((cb) => cb()))
    }

    this.keycloak.onAuthRefreshSuccess = () => {
      this.onTokenRefreshedCallbacks.forEach((cb) => cb())
    }

    this.keycloak.onAuthRefreshError = () => {
      this.onTokenExpiredCallbacks.forEach((cb) => cb())
    }
  }

  onTokenExpired(callback: TokenExpiredCallback): () => void {
    this.onTokenExpiredCallbacks.push(callback)
    return () => {
      this.onTokenExpiredCallbacks = this.onTokenExpiredCallbacks.filter((cb) => cb !== callback)
    }
  }

  onTokenRefreshed(callback: TokenRefreshedCallback): () => void {
    this.onTokenRefreshedCallbacks.push(callback)
    return () => {
      this.onTokenRefreshedCallbacks = this.onTokenRefreshedCallbacks.filter((cb) => cb !== callback)
    }
  }

  async ensureTokenValid(minValidity = 30): Promise<boolean> {
    if (!this.keycloak?.authenticated) return false
    try {
      await this.keycloak.updateToken(minValidity)
      return true
    } catch (error) {
      console.error('[AuthService] Failed to refresh token:', error)
      return false
    }
  }

  async login(): Promise<void> {
    if (!this.keycloak) await this.initialize()
    await this.keycloak!.login({ redirectUri })
  }

  async logout(): Promise<void> {
    if (!this.keycloak) return
    await this.keycloak.logout({ redirectUri: postLogoutRedirectUri })
  }

  getToken(): string | undefined {
    return this.keycloak?.token
  }

  isAuthenticated(): boolean {
    return this.keycloak?.authenticated || false
  }

  getUserProfile(): User | null {
    if (!this.keycloak?.authenticated || !this.keycloak.tokenParsed) return null
    return {
      id: this.keycloak.tokenParsed.sub || '',
      email: this.keycloak.tokenParsed.email || '',
      name: this.keycloak.tokenParsed.name || this.keycloak.tokenParsed.preferred_username || '',
      username: this.keycloak.tokenParsed.preferred_username || '',
    }
  }
}

export const authService = new AuthService()
