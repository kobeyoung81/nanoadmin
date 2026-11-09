# nanoadmin

NanoSec 平台的 admin 模块：目录、配置、审计。

## 它拥有什么

- **一组集合前缀**：`admin_`（`admin_config`、`admin_agent_types`、`admin_models`、`admin_capabilities` 等）。
  `sys_migrations` 里它只写 `service="admin"` 的记录。
- **一个 HTTP 面**：`/api/v1/config*`（本阶段只读），由门户的服务器转发；浏览器不直连它。
  `/healthz` 是唯一不带内部令牌的端点。

它**不读**别的模块的集合：用户身份、agent 归属这类事实一律走 HTTP 问对方。

## 配置

| 环境变量 | 作用 |
|---|---|
| `NANO_ADMIN_ADDR` | 监听地址，默认 `:8080` |
| `NANO_ADMIN_MONGO_URI` | 平台库连接串；未设置时回落到 `NANOSEC_BUS_DATABASE_URI` |
| `NANOSEC_BUS_INTERNAL_TOKEN` | 内部令牌；为空时所有 `/api/v1` 请求一律拒绝 |

## 命令

```bash
nanoadmin serve              # 起服务（启动时先跑一遍迁移，幂等）
nanoadmin migrate up         # 只跑迁移
nanoadmin migrate status     # 列出每个 manifest 是否已应用
nanoadmin version
```

## 阶段

P2 第一片：服务骨架 + 只读的 config 域。写入与其余目录（agent-types / models /
capabilities / prompts / permission-keys / audit）在后续切片里从 nanobus 搬来。

## 许可

GNU Affero General Public License v3.0，见 LICENSE 与 NOTICE。
