export interface User {
  id: string
  email: string
  name: string
  username: string
}

export interface AuthConfig {
  keycloak_url: string
  keycloak_realm: string
  keycloak_client_id: string
  admin_roles: string[]
}

// Mirrors internal/api's definitionResponse (workflow-server).
export interface Definition {
  tenant: string
  name: string
  buildId: string
  yaml: string
  status: string
  createdAt: string
}
