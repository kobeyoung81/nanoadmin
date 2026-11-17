# nanoadmin

目录与配置：有什么、能做什么、发生过什么。

它是八个一级服务之一。控制台里每一个编辑配置、智能体类型、模型、技能、提示词或权限键的页面，
都跟这个模块说话。

## 它是什么

- **目录。** 智能体类型与它的覆盖项、模型供应商与模型、技能、提示词块、MCP 服务、权限键 ——
  一处存放、带版本、从代码播种、在控制台编辑。
- **配置。** 每个组件的设置（`agent.*`、`runner.*`、`channels.*`、`ldap.*`、`voice.*`），
  都是有类型的键值行。
- **审计。** 谁改了什么，带着能让它有用处的细节。
- **告警。** 控制台检查并展示的规则。
- **控制台的页面。** 管理端页面在这里构建，作为插件提供给门户。

## 它在哪

| | |
|---|---|
| 集合 | `admin_config`、`admin_agent_types`、`admin_agent_type_overrides`、`admin_model_providers`、`admin_models`、`admin_skills`、`admin_user_skill_acks`、`admin_prompts`、`admin_mcp_servers`、`admin_alert_rules`、`admin_audit_log` |
| 端口 | 网络内 `8080`，宿主机 `14404`（绑 loopback） |
| 上游 | `nanoportal`（控制台转发过来），以及每个启动时读配置的服务 |
| 下游 | `nanobus` 与 `nanoprovisioner` —— 目录变了就通知它们刷新 |
| `sys_migrations` | 本服务只写自己那条 `service="admin"` |

## 快速开始

```bash
go build ./cmd/nanoadmin
NANOSEC_ADMIN_MONGO_URI="mongodb://localhost:27017/nanosec" \
NANOSEC_BUS_INTERNAL_TOKEN="…" \
  ./nanoadmin serve

./nanoadmin migrate up
./nanoadmin config seed permissions seed agent-types seed prompts seed skills seed mcp seed
```

播种是幂等且只增的：缺什么插什么，绝不覆盖运维改过的行。

## 配置

| 变量 | 含义 |
|---|---|
| `NANOSEC_ADMIN_ADDR` | 监听地址，默认 `:8080` |
| `NANOSEC_ADMIN_MONGO_URI` | 平台库；未设时回退到 `NANOSEC_BUS_DATABASE_URI` |
| `NANOSEC_BUS_INTERNAL_TOKEN` | 所有 `/api/v1` 请求都要它；`/healthz` 不要 |
| `NANOSEC_BUS_URL` / `NANOSEC_PROVISIONER_URL` | 目录变化通知到哪儿 |

## 命令行

```bash
nanoadmin serve                  # 先跑迁移，再起 HTTP 面
nanoadmin migrate up|status      # 应用清单、列出已应用
nanoadmin config get|set|seed    # 读、写、播种配置
nanoadmin permissions seed       # 权限键
nanoadmin agent-types seed       # 内建智能体类型与它们的工具体
nanoadmin prompts seed           # 基线提示词块
nanoadmin skills seed            # 内建技能
nanoadmin mcp seed               # 平台级 MCP 服务
nanoadmin version
```

## 设计决策

- **播种从不覆盖。** 内建提示词、技能、智能体类型只在缺失时插入；运维改过的那一版就留着。
  这也是播种每次启动都跑、而不是只跑一次的原因。
- **目录是通知，不是轮询。** 变更发一个事件，总线和供给服务丢掉缓存，不用等重启。
- **有些集合要按文档级解码。** 审计的 detail 是灵活文档，允许它的开关声明在集合上 ——
  配在客户端会让测试与生产不一致。
- **控制台在这里构建。** 编辑本域数据的页面，随拥有数据的模块一起发插件，而不是塞进外壳里。

## 文档

- [`docs/MANAGEMENT_PORTAL_DESIGN.md`](https://gitee.com/kobeyoung81/nanosec/blob/master/docs/MANAGEMENT_PORTAL_DESIGN.md) —— 页面与角色
- [`docs/CAPABILITY_UNIFICATION_DESIGN.md`](https://gitee.com/kobeyoung81/nanosec/blob/master/docs/CAPABILITY_UNIFICATION_DESIGN.md) —— 模型、技能、MCP 与工具如何成为一个目录
- [`docs/README.md`](https://gitee.com/kobeyoung81/nanosec/blob/master/docs/README.md) —— 全部文档索引

## 状态

随 `v0.1.0` 发布；本模块的版本钉在超仓的 `versions.lock` 里。

## 贡献与安全

开发与提交规则见
[`CONTRIBUTING.md`](https://gitee.com/kobeyoung81/nanosec/blob/master/CONTRIBUTING.md)；
漏洞请按
[`SECURITY.md`](https://gitee.com/kobeyoung81/nanosec/blob/master/SECURITY.md) 私下报告，
不要开公开 issue。

## 许可证

[AGPL-3.0](./LICENSE)。名称与标志另有说明 —— 见 [`NOTICE`](./NOTICE)。
