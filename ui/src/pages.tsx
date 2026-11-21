// The runtime snapshot, as the admin module's page.
//
// It is the portal's `/admin/runtime` screen, moved out of the shell: the shell
// mounts it at that route from /plugins/admin/, so releasing this module updates
// the screen without rebuilding the portal. The components come from the
// shell's kit through the context; the utility classes they are styled with
// come from this module's own build (see styles.css).

import './styles.css'

import { RefreshCw } from 'lucide-react'
import type { ComponentType } from 'react'
import type { PageContext } from './contract'
import type { RuntimeSnapshot } from './types'
import { useApi } from './useApi'
import { setHostContext } from './host'
import { AgentTypesPage as AgentTypesScreen } from './pages/agent-types'
import { AlertsPage as AlertsScreen } from './pages/alerts'
import { AuditPage as AuditScreen } from './pages/audit'
import { CapabilitiesPage as CapabilitiesScreen } from './pages/capabilities'
import { ComponentsPage as ComponentsScreen } from './pages/components'
import { ConfigsPage as ConfigsScreen } from './pages/configs'
import { DirectoryPage as DirectoryScreen } from './pages/directory'
import { ConversationLogsPage as ConversationsScreen } from './pages/conversations'
import { ModelsPage as ModelsScreen } from './pages/models'
import { OverviewPage as OverviewScreen } from './pages/overview'
import { VoicePage as VoiceScreen } from './pages/voice'

/**
 * withHost installs the page's context before it renders.
 *
 * A page receives the shell's services as a prop, but the components it renders are
 * nested (a provider form, a capability table, a lib module) and read them from
 * `./host`. Without this the first nested `useHost()` threw.
 */
function withHost<P extends { ctx: PageContext }>(Page: ComponentType<P>): ComponentType<P> {
  const HostedPage = (props: P) => {
    setHostContext(props.ctx)
    return <Page {...props} />
  }
  return HostedPage
}

export const AgentTypesPage = withHost(AgentTypesScreen)
export const AlertsPage = withHost(AlertsScreen)
export const AuditPage = withHost(AuditScreen)
export const CapabilitiesPage = withHost(CapabilitiesScreen)
export const ComponentsPage = withHost(ComponentsScreen)
export const ConfigsPage = withHost(ConfigsScreen)
export const DirectoryPage = withHost(DirectoryScreen)
export const ConversationLogsPage = withHost(ConversationsScreen)
export const ModelsPage = withHost(ModelsScreen)
export const OverviewPage = withHost(OverviewScreen)
export const VoicePage = withHost(VoiceScreen)

function RuntimeScreen({ ctx }: { ctx: PageContext }) {
  const { t, ui, api } = ctx
  const {
    PageHeader,
    Card,
    CardHeader,
    CardTitle,
    CardContent,
    Badge,
    Button,
    Skeleton,
    EmptyState,
    Table,
    TableHeader,
    TableBody,
    TableRow,
    TableHead,
    TableCell,
    StatusBadge,
  } = ui

  const { data, loading, error, refetch } = useApi<RuntimeSnapshot>(api, '/runtime')

  return (
    <div className="flex flex-col gap-6" data-admin-page="runtime">
      <PageHeader
        title={t('nav.runtimeLogs')}
        subtitle={t('runtime.subtitle')}
        actions={
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            <RefreshCw className="h-4 w-4" />
            {t('common.refresh')}
          </Button>
        }
      />

      {error ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-destructive">{error}</CardContent>
        </Card>
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t('runtime.containers')}</CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? (
            <Skeleton className="h-32 w-full" />
          ) : data && data.containers.length > 0 ? (
            <Table stacked>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('runtime.owner')}</TableHead>
                  <TableHead>{t('runtime.level')}</TableHead>
                  <TableHead>{t('runtime.runner')}</TableHead>
                  <TableHead>{t('runtime.status')}</TableHead>
                  <TableHead className="text-right">{t('runtime.updatedAt')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.containers.map((container) => (
                  <TableRow key={container.user_id}>
                    <TableCell label={t('runtime.owner')} className="font-medium">
                      {container.user_id}
                    </TableCell>
                    <TableCell label={t('runtime.level')}>
                      <Badge variant="secondary">{container.level}</Badge>
                    </TableCell>
                    <TableCell label={t('runtime.runner')} className="font-mono text-xs text-muted-foreground">
                      {container.runner_id || '—'}
                    </TableCell>
                    <TableCell label={t('runtime.status')}>
                      <StatusBadge status={container.status} />
                    </TableCell>
                    <TableCell label={t('runtime.updatedAt')} className="text-right text-xs text-muted-foreground">
                      {container.updated_at || t('common.na')}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <EmptyState title={t('runtime.noData')} className="py-8" />
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t('runtime.agents')}</CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? (
            <Skeleton className="h-32 w-full" />
          ) : data && data.agents.length > 0 ? (
            <Table stacked>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('runtime.agentId')}</TableHead>
                  <TableHead>{t('runtime.owner')}</TableHead>
                  <TableHead>{t('runtime.status')}</TableHead>
                  <TableHead className="text-right">{t('runtime.updatedAt')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.agents.map((agent) => (
                  <TableRow key={agent.id}>
                    <TableCell label={t('runtime.agentId')} className="font-mono text-xs font-medium">
                      {agent.id}
                    </TableCell>
                    <TableCell label={t('runtime.owner')} className="text-muted-foreground">
                      {agent.user_id}
                    </TableCell>
                    <TableCell label={t('runtime.status')}>
                      <StatusBadge status={agent.status} />
                    </TableCell>
                    <TableCell label={t('runtime.updatedAt')} className="text-right text-xs text-muted-foreground">
                      {agent.updated_at || t('common.na')}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <EmptyState title={t('runtime.noData')} className="py-8" />
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">{t('runtime.runners')}</CardTitle>
        </CardHeader>
        <CardContent>
          {loading ? (
            <Skeleton className="h-32 w-full" />
          ) : data && data.runners.length > 0 ? (
            <Table stacked>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('runtime.runner')}</TableHead>
                  <TableHead>{t('runtime.host')}</TableHead>
                  <TableHead>{t('runtime.status')}</TableHead>
                  <TableHead className="text-right">{t('runtime.lastHeartbeat')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.runners.map((runner) => (
                  <TableRow key={runner.id}>
                    <TableCell label={t('runtime.runner')} className="font-mono text-xs font-medium">
                      {runner.id}
                    </TableCell>
                    <TableCell label={t('runtime.host')} className="text-muted-foreground">
                      {runner.hostname}
                    </TableCell>
                    <TableCell label={t('runtime.status')}>
                      <StatusBadge status={runner.status} />
                    </TableCell>
                    <TableCell label={t('runtime.lastHeartbeat')} className="text-right text-xs text-muted-foreground">
                      {runner.last_heartbeat || t('common.na')}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <EmptyState title={t('runtime.noData')} className="py-8" />
          )}
        </CardContent>
      </Card>
    </div>
  )
}

export const RuntimePage = withHost(RuntimeScreen)
