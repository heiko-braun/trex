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

// Mirrors internal/manifest.Ref, as returned by GET /envelopes/{workflowID}.
export interface EnvelopeRef {
  MediaType: string
  Digest: string
  Size: number
  Bucket: string
}

// Slot name -> Ref, the full response shape of GET /envelopes/{workflowID}.
export type EnvelopeIndex = Record<string, EnvelopeRef>
