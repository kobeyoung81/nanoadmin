// The runtime snapshot the console shows: containers, their agents and the runner
// fleet. The portal assembles it (`GET /api/admin/v1/runtime`); this application
// only renders it, so the type lives here.

export type RuntimeContainer = {
  user_id: string
  level: string
  status: string
  runner_id: string
  updated_at: string
}

export type RuntimeAgent = {
  id: string
  user_id: string
  display_name: string
  status: string
  updated_at: string
}

export type RunnerSummary = {
  id: string
  hostname: string
  docker_host: string
  status: string
  last_heartbeat: string
  registered_at: string
}

export type RuntimeSnapshot = {
  containers: RuntimeContainer[]
  agents: RuntimeAgent[]
  runners: RunnerSummary[]
}

// The overview screen's payload (`GET /api/admin/v1/overview`) and the component
// table's two shapes (`/components`, `/components/{type}`). The portal assembles
// them from the modules' own answers; this application renders them.

export type LabelValue = { label: string; value: string }

export type AlertSummary = {
  rule_id: string
  severity: string
  message: string
}

export type Overview = {
  system_health: string
  summary: {
    active_users: number
    running_agents: number
    pending_inbound: number
    pending_outbound: number
    active_containers: number
  }
  tier_distribution: Record<string, number>
  top_alerts: AlertSummary[]
}

export type ComponentSummary = {
  type: string
  display_name: string
  status: string
  detail: string
  metrics: LabelValue[]
  updated_at: string
}

export type ComponentDetail = {
  type: string
  display_name: string
  status: string
  description: string
  config_version: string
  updated_at: string
  metrics: LabelValue[]
  instances: {
    id: string
    name: string
    status: string
    last_seen_at: string
    detail: string
  }[]
}

// The audit log's entry, the conversation log's two shapes and the alert rule.
// The portal serves them from the modules that own the data; this application
// renders them.

export type AuditEntry = {
  id: number
  actor_id: string
  /** The actor's display name, resolved when the account still exists. */
  actor_name?: string
  actor_role: string
  action: string
  resource_type: string
  resource_id: string
  /** The display name behind resource_id for agents, users and groups. */
  resource_name?: string
  detail: Record<string, unknown> | null
  ip: string
  created_at: string
}

export type ConversationSummary = {
  id: string
  agent_id: string
  agent_name?: string
  account_id: string
  account_name?: string
  channel: string
  chat_id: string
  status: string
  message_count: number
  messages_bytes: number
  updated_at: string
}

export type ConversationDetail = ConversationSummary & {
  summary: string
  messages: unknown
}

export type AlertRule = {
  id: number
  severity: string
  message: string
  metric: string
  threshold: number
  enabled: boolean
}

// The configuration centre's two shapes: one entry per component and one
// component's key/value table. `/me` answers the role the write rule reads.
export type ConfigSummary = {
  component_type: string
  updated_at: string
  source?: string
}

export type ConfigDetail = {
  component_type: string
  updated_at: string
  config: Record<string, unknown>
}

export type AdminIdentity = {
  user_id: string
  display_name: string
  role: string
  tier?: string
}
