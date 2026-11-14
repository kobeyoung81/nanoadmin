// The console's alert rules, as the admin module's page.
//
// A rule is a message, a severity and a metric threshold; the list is the module's
// own data (admin_alert_rules) and the create/delete calls go through the console's
// API. The form is inline: three fields do not need a page of their own.

import { useState, type FormEvent } from 'react'
import { BellPlus, RefreshCw, Trash2 } from 'lucide-react'
import type { PageContext } from '../contract'
import type { AlertRule } from '../types'
import { useApi } from '../useApi'

const SEVERITIES = ['info', 'warning', 'critical'] as const
const SEVERITY_VARIANT: Record<string, 'info' | 'warning' | 'danger' | 'secondary'> = {
  info: 'info',
  warning: 'warning',
  critical: 'danger',
}

/** useAsyncAction wraps a write with a busy flag; the toasts stay at the call site. */
function useAsyncAction() {
  const [busy, setBusy] = useState(false)
  const run = async (action: () => Promise<unknown>) => {
    setBusy(true)
    try {
      return await action()
    } finally {
      setBusy(false)
    }
  }
  return { run, busy }
}

export function AlertsPage({ ctx }: { ctx: PageContext }) {
  const { t, ui, api, toast } = ctx
  const {
    PageHeader,
    Card,
    CardContent,
    Badge,
    Button,
    Input,
    Select,
    SelectContent,
    SelectItem,
    SelectTrigger,
    SelectValue,
    Switch,
    FormField,
    Skeleton,
    EmptyState,
    ConfirmDialog,
    Dialog,
    DialogContent,
    DialogDescription,
    DialogFooter,
    DialogHeader,
    DialogTitle,
    Table,
    TableHeader,
    TableBody,
    TableRow,
    TableHead,
    TableCell,
    Tooltip,
    TooltipTrigger,
    TooltipContent,
  } = ui

  const { data, loading, error, refetch, mutate } = useApi<AlertRule[]>(api, '/alert-rules')
  const [createOpen, setCreateOpen] = useState(false)
  const [pendingDelete, setPendingDelete] = useState<AlertRule | null>(null)
  const { run, busy } = useAsyncAction()

  const handleDelete = async (rule: AlertRule) => {
    await run(async () => {
      await api.del(`/alert-rules/${rule.id}`)
      mutate((data ?? []).filter((item) => item.id !== rule.id))
      toast.success(t('alerts.deletedRule', { message: rule.message }))
    }).catch((err: Error) => toast.error(err.message))
  }

  const handleCreate = async (rule: Omit<AlertRule, 'id'>) => {
    try {
      await run(async () => {
        const created = await api.post<{ id: number }>('/alert-rules', rule)
        mutate([...(data ?? []), { ...rule, id: created.id }])
        toast.success(t('alerts.createdRule', { message: rule.message }))
      })
      return true
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t('alerts.createFailed'))
      return false
    }
  }

  return (
    <div className="flex flex-col gap-6" data-admin-page="alerts">
      <PageHeader
        title={t('nav.alertRules')}
        subtitle={t('alerts.subtitle')}
        actions={
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => {
                refetch()
                toast.success(t('alerts.refreshed'))
              }}
            >
              <RefreshCw className="h-4 w-4" />
              {t('common.refresh')}
            </Button>
            <Button size="sm" onClick={() => setCreateOpen(true)}>
              <BellPlus className="h-4 w-4" />
              {t('alerts.newRule')}
            </Button>
          </div>
        }
      />

      <Card>
        <CardContent className="pt-6">
          {error ? <p className="pb-4 text-sm text-destructive">{error}</p> : null}

          {loading ? (
            <div className="space-y-2">
              {Array.from({ length: 4 }).map((_, index) => (
                <Skeleton key={index} className="h-14 w-full" />
              ))}
            </div>
          ) : data && data.length > 0 ? (
            <Table stacked>
              <TableHeader>
                <TableRow>
                  <TableHead>{t('configs.value')}</TableHead>
                  <TableHead>{t('alerts.severity')}</TableHead>
                  <TableHead>{t('alerts.metric')}</TableHead>
                  <TableHead className="text-right">{t('alerts.threshold')}</TableHead>
                  <TableHead>{t('alerts.enabled')}</TableHead>
                  <TableHead className="w-16 text-right" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {data.map((rule) => (
                  <TableRow key={rule.id}>
                    <TableCell label={t('configs.value')} className="font-medium">
                      {rule.message}
                    </TableCell>
                    <TableCell label={t('alerts.severity')}>
                      <Badge variant={SEVERITY_VARIANT[rule.severity] ?? 'secondary'}>
                        {t(`enums.severity.${rule.severity}`, { defaultValue: rule.severity })}
                      </Badge>
                    </TableCell>
                    <TableCell label={t('alerts.metric')} className="font-mono text-xs text-muted-foreground">
                      {rule.metric || '—'}
                    </TableCell>
                    <TableCell label={t('alerts.threshold')} className="text-right tabular-nums">
                      {rule.threshold}
                    </TableCell>
                    <TableCell label={t('alerts.enabled')}>
                      <Switch checked={rule.enabled} aria-readonly />
                    </TableCell>
                    <TableCell label={t('account.actions')} className="text-right">
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-7 w-7 text-muted-foreground hover:text-destructive"
                            onClick={() => setPendingDelete(rule)}
                            aria-label={t('alerts.deleteNamedRule', { message: rule.message })}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>{t('alerts.deleteRule')}</TooltipContent>
                      </Tooltip>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          ) : (
            <EmptyState
              icon={BellPlus}
              title={t('alerts.noRulesTitle')}
              description={t('alerts.noRulesDescription')}
              action={
                <Button size="sm" onClick={() => setCreateOpen(true)}>
                  {t('alerts.newRule')}
                </Button>
              }
              className="py-12"
            />
          )}
        </CardContent>
      </Card>

      <CreateRuleDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        busy={busy}
        onCreate={handleCreate}
        t={t}
        ui={{ Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter, FormField, Select, SelectTrigger, SelectValue, SelectContent, SelectItem, Input, Switch, Button }}
      />

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => !open && setPendingDelete(null)}
        title={t('alerts.deleteTitle')}
        description={
          pendingDelete
            ? t('alerts.deleteDescription', {
                message: pendingDelete.message,
                severity: t(`enums.severity.${pendingDelete.severity}`, { defaultValue: pendingDelete.severity }),
              })
            : undefined
        }
        busy={busy}
        onConfirm={async () => {
          if (pendingDelete) {
            await handleDelete(pendingDelete)
          }
        }}
      />
    </div>
  )
}

type RuleDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  busy: boolean
  onCreate: (rule: Omit<AlertRule, 'id'>) => Promise<boolean>
  t: PageContext['t']
  ui: Pick<
    PageContext['ui'],
    | 'Dialog'
    | 'DialogContent'
    | 'DialogHeader'
    | 'DialogTitle'
    | 'DialogDescription'
    | 'DialogFooter'
    | 'FormField'
    | 'Select'
    | 'SelectTrigger'
    | 'SelectValue'
    | 'SelectContent'
    | 'SelectItem'
    | 'Input'
    | 'Switch'
    | 'Button'
  >
}

// CreateRuleDialog is a module-level component: a component created during render
// is a new type every render, and React remounts its subtree (the dialog's inputs
// would lose their focus on every keystroke).
function CreateRuleDialog({ open, onOpenChange, busy, onCreate, t, ui }: RuleDialogProps) {
  const { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } = ui
  const { FormField, Select, SelectTrigger, SelectValue, SelectContent, SelectItem, Input, Switch, Button } = ui
  const [severity, setSeverity] = useState<string>('warning')
  const [message, setMessage] = useState('')
  const [metric, setMetric] = useState('')
  const [threshold, setThreshold] = useState('0')
  const [enabled, setEnabled] = useState(true)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!message.trim()) {
      return
    }
    const created = await onCreate({
      severity,
      message: message.trim(),
      metric: metric.trim(),
      threshold: Number(threshold) || 0,
      enabled,
    })
    if (created) {
      setMessage('')
      setMetric('')
      setThreshold('0')
      setEnabled(true)
      onOpenChange(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t('alerts.createTitle')}</DialogTitle>
          <DialogDescription>{t('alerts.createDescription')}</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} className="flex flex-col gap-4">
          <FormField label={t('alerts.severity')}>
            <Select value={severity} onValueChange={setSeverity}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {SEVERITIES.map((level) => (
                  <SelectItem key={level} value={level}>
                    {t(`enums.severity.${level}`, { defaultValue: level })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </FormField>
          <FormField label={t('alerts.messageLabel')}>
            <Input
              value={message}
              onChange={(event) => setMessage(event.target.value)}
              placeholder={t('alerts.messagePlaceholder')}
            />
          </FormField>
          <FormField label={t('alerts.metric')} hint={t('alerts.metricHint')}>
            <Input
              value={metric}
              onChange={(event) => setMetric(event.target.value)}
              placeholder={t('alerts.metricPlaceholder')}
            />
          </FormField>
          <FormField label={t('alerts.threshold')}>
            <Input type="number" value={threshold} onChange={(event) => setThreshold(event.target.value)} />
          </FormField>
          <label className="flex items-center justify-between rounded-lg border p-3">
            <span className="text-sm font-medium">{t('alerts.enabled')}</span>
            <Switch checked={enabled} onCheckedChange={setEnabled} />
          </label>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>
              {t('common.cancel')}
            </Button>
            <Button type="submit" disabled={busy}>
              {t('alerts.createRule')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
