// Package seed holds the platform's default configuration.
//
// These are the config domain's defaults, so they live with the service that
// owns the collection. The two voice strings duplicate constants in
// nanobus/internal/voice (its runtime fallbacks) — keep them in step.
package seed

import "gitee.com/kobeyoung81/nanoadmin/internal/store"

func ci(key, value, vtype, desc string) store.ConfigItem {
	return store.ConfigItem{Key: key, Value: value, ValueType: vtype, Description: desc}
}

func DefaultConfigItems() map[string][]store.ConfigItem {
	return map[string][]store.ConfigItem{
		"bus": {
			ci("host", "0.0.0.0", "string", "Server bind address"),
			ci("port", "9000", "int", "Server port"),
		},
		"gateway": {
			ci("channels.cli.enabled", "true", "bool", "Enable CLI channel"),
			ci("channels.cli.userID", "default", "string", "Default CLI user ID"),
			ci("channels.wecom.enabled", "false", "bool", "Enable WeCom channel"),
			ci("channels.wecom.corpID", "", "secret", "WeCom Corp ID"),
			ci("channels.wecom.corpSecret", "", "secret", "WeCom Corp Secret"),
			ci("channels.wecom.agentID", "0", "int", "WeCom Agent ID"),
			ci("channels.wecom.token", "", "secret", "WeCom callback token"),
			ci("channels.wecom.encodingAESKey", "", "secret", "WeCom AES key"),
			ci("channels.wecom.callbackAddr", ":8081", "string", "WeCom callback listen address"),
			ci("channels.feishu.enabled", "false", "bool", "Enable Feishu channel"),
			ci("channels.feishu.appId", "", "secret", "Feishu App ID"),
			ci("channels.feishu.appSecret", "", "secret", "Feishu App Secret"),
			ci("channels.feishu.encryptKey", "", "secret", "Feishu encrypt key"),
			ci("channels.feishu.verificationToken", "", "secret", "Feishu verification token"),
			ci("channels.wechatkf.enabled", "false", "bool", "Enable WeChat KF channel"),
			ci("channels.wechatkf.corpID", "", "secret", "WeChat KF Corp ID"),
			ci("channels.wechatkf.kfSecret", "", "secret", "WeChat KF secret"),
			ci("channels.wechatkf.openKfID", "", "string", "WeChat KF open kf ID"),
			ci("channels.wechatkf.token", "", "secret", "WeChat KF callback token"),
			ci("channels.wechatkf.encodingAESKey", "", "secret", "WeChat KF AES key"),
			ci("channels.wechatkf.callbackAddr", ":8082", "string", "WeChat KF callback listen address"),
			ci("channels.web.enabled", "true", "bool", "Enable browser web chat channel"),
			ci("channels.web.listenAddr", ":8080", "string", "Web chat listen address"),
			// Channel messaging capabilities — read by the gateway (delivery)
			// and the agent (output adaptation).
			ci("channels.web.streaming", "true", "bool", "Web renders tokens incrementally"),
			ci("channels.web.markdown", "true", "bool", "Web renders markdown"),
			ci("channels.wecom.streaming", "false", "bool", "WeCom delivers full messages (no streaming)"),
			ci("channels.wecom.markdown", "true", "bool", "WeCom renders markdown"),
			ci("channels.wecom.maxContentLen", "0", "int", "WeCom max content length (0 = unlimited)"),
			ci("channels.wechatkf.streaming", "false", "bool", "WeChat KF delivers full messages (no streaming)"),
			ci("channels.wechatkf.markdown", "false", "bool", "WeChat KF is plain text only"),
			ci("channels.wechatkf.maxContentLen", "4000", "int", "WeChat KF max content length"),
			ci("channels.feishu.streaming", "false", "bool", "Feishu delivers full messages (no streaming)"),
			ci("channels.feishu.markdown", "true", "bool", "Feishu renders markdown"),
			ci("ack.enabled", "false", "bool", "Enable auto-acknowledgment"),
			ci("ack.text", "", "string", "Auto-acknowledgment text"),
		},
		"runner": {
			ci("runner.runnerID", "nanorunner", "string", "Stable runner identity reused across restarts"),
			ci("runner.pollInterval", "5", "int", "Poll interval in seconds"),
			ci("runner.backend", "docker", "string", "Provisioner backend: docker | swarm | k8s"),
			ci("agentImage", "kobeyoung81/nanosec:latest", "string", "Docker image for agent containers"),
			ci("network", "myriadmind_default", "string", "Docker network name"),
			ci("seccompProfile", "", "string", "Daemon-host path to the agent container seccomp profile (empty = Docker default)"),
			// Without a TZ the agent container runs in UTC: dates in the system
			// prompt, cron schedules and git timestamps are all off for a CST
			// deployment. Seeded so an operator can change it in the config center;
			// the runner re-reads it before every provisioning.
			ci("runner.agentTimezone", "Asia/Shanghai", "string", "IANA timezone injected as TZ into agent containers (empty = UTC)"),
			ci("runner.quota.free.memory", "512", "int", "Free tier memory limit (MB)"),
			ci("runner.quota.free.cpu", "1000000000", "int", "Free tier CPU limit (nanoCPUs)"),
			ci("runner.quota.pro.memory", "2048", "int", "Pro tier memory limit (MB)"),
			ci("runner.quota.pro.cpu", "2000000000", "int", "Pro tier CPU limit (nanoCPUs)"),
			ci("runner.quota.ultra.memory", "4096", "int", "Ultra tier memory limit (MB)"),
			ci("runner.quota.ultra.cpu", "4000000000", "int", "Ultra tier CPU limit (nanoCPUs)"),
		},
		"auth": {
			ci("jwksUrl", "http://nanoauth:8081/auth/v1/.well-known/jwks", "string", "nanoauth JWKS endpoint"),
			// Bootstrap administrator: the first account created through a sign-up
			// takes the administrator role while the installation has none. Turn it
			// off before exposing registration to an untrusted network.
			ci("firstUserIsAdmin", "true", "bool", "The first account to register becomes the platform administrator"),
			// LDAP directory login: users sign in with their corporate email and the
			// directory verifies the password. Values below are the Active Directory
			// defaults; nanoauth/README.md lists the OpenLDAP equivalents.
			ci("ldap.enabled", "false", "bool", "Enable LDAP directory login"),
			ci("ldap.url", "ldap://127.0.0.1:389", "string", "Directory URL (ldap:// or ldaps://)"),
			ci("ldap.startTLS", "false", "bool", "Upgrade the connection with StartTLS"),
			ci("ldap.tlsInsecureSkipVerify", "false", "bool", "Skip TLS certificate verification"),
			ci("ldap.caCertPEM", "", "secret", "Custom CA certificate (PEM) for the directory"),
			ci("ldap.timeoutSeconds", "10", "int", "Connect, search and bind timeout (seconds)"),
			ci("ldap.baseDN", "", "string", "Search base DN, e.g. dc=corp,dc=local"),
			ci("ldap.userFilter", "(&(objectClass=user)(mail=%s))", "string", "User search filter; %s is the escaped email"),
			ci("ldap.requiredGroupDN", "", "string", "Only members of this group may sign in (empty = anyone)"),
			ci("ldap.groupMatchInChain", "false", "bool", "Resolve nested groups (AD rule 1.2.840.113556.1.4.1941)"),
			ci("ldap.bindDN", "", "string", "Search service account DN (empty = anonymous search)"),
			ci("ldap.bindPassword", "", "secret", "Search service account password"),
			ci("ldap.attrUniqueId", "objectGUID", "string", "Stable account key attribute (OpenLDAP: entryUUID)"),
			ci("ldap.attrUsername", "sAMAccountName", "string", "Fallback account key attribute (OpenLDAP: uid)"),
			ci("ldap.attrDisplayName", "displayName", "string", "Display name attribute (OpenLDAP: cn)"),
			ci("ldap.autoProvision", "true", "bool", "Provision a platform account on the first successful login"),
			// Account relationship policies, editable on the portal's directory
			// login page alongside the keys above.
			ci("ldap.linkPolicy", "auto", "string", "Email collision with an existing local account: auto = link it, refuse = reject the login"),
			ci("ldap.disableOnMissing", "false", "bool", "Suspend a directory-linked account when the entry disappears from the directory"),
			ci("ldap.syncDisplayName", "false", "bool", "Refresh the platform display name from the directory on each login"),
		},
		"agent": {
			// Platform LLM configuration lives in the model catalog
			// (model_providers/models, scope='platform'); see migration 041.
			// Runtime policy/tool settings (loop limits, summarization, memory,
			// session lifecycle, prompt, tool policy) live on AgentTypes and
			// per-agent overrides; see migration 057.
			// Execution mode a scheduled task runs in when neither the task nor
			// the agent's own setting says otherwise (cron.TaskMode).
			ci("agent.scheduledTaskMode", "build", "string", "Execution mode scheduled tasks run in: build | plan"),
			// The platform half of the language setting. The runtime states it in
			// the per-turn language note; a user may override it in their own
			// preferences (the user-level half).
			ci("agent.language.default", "zh", "string", "Language replies are written in unless the user overrides it (zh, en, ja, ko, ru, ar, fr, de, es, pt)"),
			// Subagent and summarizer calls carry their own reasoning effort,
			// separate from the agent's main loop. Empty means "send no level at
			// all", which lets the model use its own default.
			ci("agent.llmSubagentReasoningEffort", "", "string", "Reasoning effort for subagent calls: empty = send none (the model decides); none|low|medium|high"),
			ci("agent.llmSummarizeReasoningEffort", "", "string", "Reasoning effort for summarization calls: empty = send none; none|low|medium|high"),
			ci("media.corpID", "", "secret", "WeCom media Corp ID"),
			ci("media.corpSecret", "", "secret", "WeCom media Corp Secret"),
			ci("media.agentID", "0", "int", "WeCom media Agent ID"),
			ci("paths.dataRoot", "~/.nanosec", "string", "Persistent agent data root"),
			// paths.* are derived per agent under agents/<agentID>/agent and
			// agents/<agentID>/workspace when left empty.
			ci("paths.sessionDir", "", "string", "Per-agent session directory (empty = derive)"),
			ci("paths.cronFile", "", "string", "Per-agent cron state file (empty = derive)"),
			ci("paths.mediaBaseDir", "", "string", "Per-agent media directory (empty = derive)"),
			ci("paths.workspaceDir", "", "string", "Per-agent working files root (empty = derive)"),
			ci("paths.identityFile", "", "string", "Per-agent AGENT.md path (empty = derive)"),
			ci("paths.bootstrapDir", "", "string", "Per-agent bootstrap directory (empty = derive)"),
			ci("paths.memoryDir", "", "string", "Per-agent memory directory (empty = derive)"),
			ci("paths.skillsDir", "", "string", "Per-agent skills directory (empty = derive)"),
		},
		// Voice input for the browser composer: one platform-wide setting set,
		// no per-user override, in three tiers — realtime (what the composer
		// streams into), batch (a second pass over the whole take by an external
		// file-transcription model) and polish (a chat model that rewrites the
		// winning text). See docs/VOICE_INPUT_DESIGN.md.
		"voice": {
			ci("enabled", "false", "bool", "Enable dictation in the composer"),
			ci("shortcut", "Alt+M", "string", "Key combination that starts and stops a dictation; empty = none. Each user may override it in their own browser (e.g. Alt+M)"),
			ci("realtime.engine", "local", "string", "Realtime tier: local = the platform's own ASR service, remote = an external realtime provider"),
			ci("realtime.providerId", "", "int", "Model-catalog provider (platform scope) serving the realtime tier"),
			ci("realtime.model", "", "string", "Model alias for realtime transcription"),
			ci("realtime.language", "zh", "string", "Recognition language hint (empty = let the model detect it)"),
			ci("realtime.prompt", "", "string", "Keyword/term hint for a remote realtime model; the local engine ignores it"),
			ci("realtime.maxSeconds", "120", "int", "Longest single dictation (seconds)"),
			ci("realtime.commitIntervalMs", "0", "int", "Commit every N ms of audio; 0 = commit when the user stops"),
			ci("realtime.audioRate", "24000", "int", "Capture rate the composer uses; 24000 is the OpenAI realtime contract, a local 16 kHz model wants 16000"),
			ci("realtime.hotwords", "", "string", "Terms the local engine biases towards, one per line (it rebuilds its recognisers when this changes)"),
			ci("batch.enabled", "false", "bool", "Re-transcribe the whole take with an external model when it ends"),
			ci("batch.trigger", "always", "string", "When the batch pass runs: always | manual | empty (empty or suspiciously short live transcript)"),
			ci("batch.providerId", "", "int", "Model-catalog provider (platform scope) serving the batch tier"),
			ci("batch.model", "", "string", "Model alias for the batch transcription"),
			ci("batch.language", "zh", "string", "Language hint for the batch model (empty = let it detect)"),
			ci("batch.prompt", "", "string", "Keyword/term hint handed to the batch model"),
			ci("polish.enabled", "true", "bool", "Run the transcript through a chat model before filling the box"),
			ci("polish.modelId", "", "int", "Model-catalog chat model (platform scope) used to polish"),
			ci("polish.prompt", "你是语音输入的后处理助手。把用户口述的文本整理成可以直接发送的消息："+
				"补全标点、纠正明显的同音与识别错字、去掉口头语与重复，保留原意、语气和语言（中文就用中文）。"+
				"不要添加原文没有的信息，不要解释，不要加引号或代码块，只输出整理后的文本。", "string", "System prompt for the polish pass"),
			ci("polish.thinking", "none", "string", "Reasoning effort for the polish model: none = off (sent explicitly), else low|medium|high"),
			ci("polish.skipAfterBatch", "false", "bool", "Skip the polish pass when the batch model already rewrote the text"),
		},
	}
}
