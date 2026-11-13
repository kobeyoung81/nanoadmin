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
