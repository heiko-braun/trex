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

// Mirrors internal/api's discoveredAgentResponse (workflow-server).
export interface DiscoveredAgent {
  id: string
  name: string
  type: string
}

// Mirrors internal/api's registeredAgentResponse (workflow-server).
export interface RegisteredAgent {
  id: string
  name: string
  taskQueue: string
  running: boolean
}
