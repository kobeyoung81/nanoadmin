# nanoadmin

The catalogue and the configuration: what exists, what it is allowed to do, and what
happened.

It is one of the eight first-class services. Every console screen that edits
configuration, an agent type, a model, a skill, a prompt or a permission key talks to
this module.

## What it is

- **The catalogue.** Agent types and their overrides, model providers and models,
  skills, prompt blocks, MCP servers and permission keys — one place, versioned, seeded
  from code and edited in the console.
- **Configuration.** Every component's settings (`agent.*`, `runner.*`,
  `channels.*`, `ldap.*`, `voice.*`) as typed key-value rows.
- **The audit trail.** Who changed what, with the detail that makes it useful.
- **Alerts.** Rules the console evaluates and shows.
- **The console's screens.** The admin pages are built here and served to the portal
  as a plugin.

## Where it sits

| | |
|---|---|
| Collections | `admin_config`, `admin_agent_types`, `admin_agent_type_overrides`, `admin_model_providers`, `admin_models`, `admin_skills`, `admin_user_skill_acks`, `admin_prompts`, `admin_mcp_servers`, `admin_alert_rules`, `admin_audit_log` |
| Port | `8080` in the network, `14404` on the host (loopback) |
| Upstream | `nanoportal` (the console forwards to it), and every service that reads configuration at startup |
| Downstream | `nanobus` and `nanoprovisioner` — a changed catalogue entry tells them to refresh |
| `sys_migrations` | this service writes only its own `service="admin"` rows |

## Quick start

```bash
go build ./cmd/nanoadmin
NANOSEC_ADMIN_MONGO_URI="mongodb://localhost:27017/nanosec" \
NANOSEC_BUS_INTERNAL_TOKEN="…" \
  ./nanoadmin serve

./nanoadmin migrate up
./nanoadmin config seed permissions seed agent-types seed prompts seed skills seed mcp seed
```

Seeding is idempotent and additive: it inserts what is missing and never overwrites a
row an operator edited.

## Configuration

| Variable | Meaning |
|---|---|
| `NANOSEC_ADMIN_ADDR` | Listen address, default `:8080` |
| `NANOSEC_ADMIN_MONGO_URI` | The platform database; falls back to `NANOSEC_BUS_DATABASE_URI` |
| `NANOSEC_BUS_INTERNAL_TOKEN` | Every `/api/v1` request needs it; `/healthz` does not |
| `NANOSEC_BUS_URL` / `NANOSEC_PROVISIONER_URL` | Where a catalogue change is announced |

## CLI

```bash
nanoadmin serve                  # migrations, then the HTTP surface
nanoadmin migrate up|status      # apply the manifests, list what is applied
nanoadmin config get|set|seed    # read, write, seed configuration
nanoadmin permissions seed       # the permission keys
nanoadmin agent-types seed       # built-in agent types and their belts
nanoadmin prompts seed           # the baseline prompt blocks
nanoadmin skills seed            # bundled skills
nanoadmin mcp seed               # platform MCP servers
nanoadmin version
```

## Design decisions

- **Seeding never overwrites.** A bundled prompt, skill or agent type is inserted when
  absent; the version an operator edited stays edited. That is why the seeder runs on
  every start instead of once.
- **The catalogue is announced, not polled.** A change publishes an event so the bus
  and the provisioner drop their cached copy instead of waiting for a restart.
- **Some collections carry a document-level decode.** The audit detail is stored as a
  flexible document, and the setting that allows it is declared on the collection —
  setting it on the client would make tests disagree with production.
- **The console is built here.** Screens that edit this domain ship as a plugin with
  the module that owns the data, not inside the shell.

## Docs

- [`docs/MANAGEMENT_PORTAL_DESIGN.md`](https://gitee.com/kobeyoung81/nanosec/blob/master/docs/MANAGEMENT_PORTAL_DESIGN.md) — the screens and roles
- [`docs/CAPABILITY_UNIFICATION_DESIGN.md`](https://gitee.com/kobeyoung81/nanosec/blob/master/docs/CAPABILITY_UNIFICATION_DESIGN.md) — how models, skills, MCP and tools became one catalogue
- [`docs/README.md`](https://gitee.com/kobeyoung81/nanosec/blob/master/docs/README.md) — every document

## Status

Released as part of `v0.1.0`; the version of this module is pinned in the
super-repo's `versions.lock`.

## Contributing and security

[`CONTRIBUTING.md`](https://gitee.com/kobeyoung81/nanosec/blob/master/CONTRIBUTING.md)
for setup and commit rules; report vulnerabilities privately through
[`SECURITY.md`](https://gitee.com/kobeyoung81/nanosec/blob/master/SECURITY.md), never
in a public issue.

## License

[AGPL-3.0](./LICENSE). The name and logo are covered separately — see
[`NOTICE`](./NOTICE).
