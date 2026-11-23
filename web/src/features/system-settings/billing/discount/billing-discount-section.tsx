/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useQuery } from '@tanstack/react-query'
import { BadgePercent, Edit, Plus, Trash2 } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/confirm-dialog'
import {
  DataTableRowActionMenu,
  StaticDataTable,
  TruncatedCell,
} from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  DropdownMenuGroup,
  DropdownMenuItem,
} from '@/components/ui/dropdown-menu'
import { Switch } from '@/components/ui/switch'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { toIntlLocale } from '@/i18n/languages'

import { SettingsPageFormActions } from '../../components/settings-page-context'
import { SettingsSection } from '../../components/settings-section'
import { useUpdateOption } from '../../hooks/use-update-option'
import {
  DISCOUNT_RULES_OPTION_KEY,
  getDiscountChannels,
  parseDiscountRules,
  type DiscountRule,
  type DiscountSchedule,
} from './api'
import { DiscountRuleEditorDialog } from './rule-editor-dialog'
import { getWeekdayLabels } from './weekday-labels'

interface Props {
  rulesJson: string
}

export function BillingDiscountSection(props: Props) {
  const { t, i18n } = useTranslation()
  const updateOption = useUpdateOption()
  const savedRules = useMemo(
    () => parseDiscountRules(props.rulesJson),
    [props.rulesJson]
  )
  const [rules, setRules] = useState<DiscountRule[]>(savedRules)
  const [editingIndex, setEditingIndex] = useState<number | null>(null)
  const [editorOpen, setEditorOpen] = useState(false)
  const [deletingIndex, setDeletingIndex] = useState<number | null>(null)

  useEffect(() => {
    setRules(savedRules)
  }, [savedRules])

  // Only fetch channels when a rule targets some: rules without channel
  // conditions do not need the list.
  const hasChannelConditions = rules.some((rule) => rule.channel_ids.length > 0)
  const channelsQuery = useQuery({
    queryKey: ['billing-discount-channels'],
    queryFn: getDiscountChannels,
    enabled: hasChannelConditions,
  })
  const channelNames = useMemo(() => {
    const names = new Map<string, string>()
    for (const channel of channelsQuery.data?.data ?? []) {
      names.set(String(channel.id), channel.name)
    }
    return names
  }, [channelsQuery.data])

  const isDirty = JSON.stringify(rules) !== JSON.stringify(savedRules)

  const weekdayLabels = useMemo(
    () =>
      getWeekdayLabels(toIntlLocale(i18n.resolvedLanguage || i18n.language)),
    [i18n.language, i18n.resolvedLanguage]
  )

  function describeSchedule(schedule: DiscountSchedule | null): string {
    if (!schedule) return t('Always active')
    const time =
      schedule.start_time && schedule.end_time
        ? `${schedule.start_time}–${schedule.end_time}`
        : ''
    if (schedule.month_days.length > 0) {
      const days = schedule.month_days.join(', ')
      return time
        ? t('Monthly on day {{days}}, {{time}}', { days, time })
        : t('Monthly on day {{days}}', { days })
    }
    if (schedule.weekdays.length > 0) {
      const days = schedule.weekdays
        .map((day) => weekdayLabels[day] ?? String(day))
        .join(', ')
      return time
        ? t('Weekly on {{days}}, {{time}}', { days, time })
        : t('Weekly on {{days}}', { days })
    }
    return time ? t('Daily {{time}}', { time }) : t('Always active')
  }

  function openEditor(index: number | null) {
    setEditingIndex(index)
    setEditorOpen(true)
  }

  function handleSaveRule(rule: DiscountRule) {
    if (editingIndex === null) {
      setRules([...rules, rule])
      return
    }
    setRules(rules.map((item, i) => (i === editingIndex ? rule : item)))
  }

  function toggleRule(index: number, enabled: boolean) {
    setRules(
      rules.map((item, i) => (i === index ? { ...item, enabled } : item))
    )
  }

  async function handleSubmit() {
    await updateOption.mutateAsync({
      key: DISCOUNT_RULES_OPTION_KEY,
      value: JSON.stringify(rules),
    })
  }

  const indexedRules = rules.map((rule, index) => ({ rule, index }))

  return (
    <SettingsSection title={t('Discount Management')}>
      <SettingsPageFormActions
        onSave={handleSubmit}
        isSaving={updateOption.isPending}
        isSaveDisabled={!isDirty}
        saveLabel='Save discount rules'
      />
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <p className='text-muted-foreground text-sm'>
          {t(
            'Matched requests are billed at final cost × ratio. When several rules match, the lowest ratio wins.'
          )}
        </p>
        <Button variant='outline' onClick={() => openEditor(null)}>
          <Plus aria-hidden='true' />
          {t('Add discount rule')}
        </Button>
      </div>

      {rules.length === 0 ? (
        <EmptyState
          icon={BadgePercent}
          title={t('No discount rules yet')}
          description={t(
            'Add a rule to discount selected users, channels or request User-Agents.'
          )}
          className='min-h-56'
        />
      ) : (
        <StaticDataTable
          className='overflow-x-auto'
          tableClassName='min-w-[1180px] table-fixed [&_th]:px-4 [&_td]:px-4 [&_td]:py-3'
          data={indexedRules}
          getRowKey={(item) => `${item.index}:${item.rule.name}`}
          columns={[
            {
              id: 'enabled',
              header: t('Enabled'),
              className: 'w-20',
              cell: (item) => (
                <Switch
                  checked={item.rule.enabled}
                  aria-label={t('Enable {{name}}', { name: item.rule.name })}
                  onCheckedChange={(enabled) => toggleRule(item.index, enabled)}
                />
              ),
            },
            {
              id: 'name',
              header: t('Name'),
              className: 'w-[16%]',
              cell: (item) => (
                <TruncatedCell tabIndex={0}>{item.rule.name}</TruncatedCell>
              ),
            },
            {
              id: 'ratio',
              header: t('Discount ratio'),
              className: 'w-28',
              cellClassName: 'tabular-nums',
              cell: (item) => `× ${item.rule.ratio}`,
            },
            {
              id: 'schedule',
              header: t('Active window'),
              className: 'w-[18%]',
              cell: (item) => {
                const schedule = item.rule.schedule
                return (
                  <TruncatedCell
                    tabIndex={0}
                    tooltipContent={[
                      describeSchedule(schedule),
                      schedule?.timezone
                        ? t('Timezone: {{timezone}}', {
                            timezone: schedule.timezone,
                          })
                        : undefined,
                    ]
                      .filter(Boolean)
                      .join('\n')}
                  >
                    {describeSchedule(schedule)}
                  </TruncatedCell>
                )
              },
            },
            {
              id: 'users',
              header: t('Users'),
              className: 'w-[14%]',
              cell: (item) => (
                <TruncatedCell
                  tabIndex={0}
                  tooltipContent={item.rule.user_ids
                    .map((id) => `#${id}`)
                    .join(', ')}
                >
                  {item.rule.user_ids.length > 0
                    ? t('{{count}} users', { count: item.rule.user_ids.length })
                    : '—'}
                </TruncatedCell>
              ),
            },
            {
              id: 'channels',
              header: t('Channels'),
              className: 'w-[14%]',
              cell: (item) => (
                <TruncatedCell
                  tabIndex={0}
                  tooltipContent={item.rule.channel_ids
                    .map((id) => channelNames.get(String(id)) ?? `#${id}`)
                    .join(', ')}
                >
                  {item.rule.channel_ids.length > 0
                    ? t('{{count}} channels', {
                        count: item.rule.channel_ids.length,
                      })
                    : '—'}
                </TruncatedCell>
              ),
            },
            {
              id: 'user-agents',
              header: t('User-Agent'),
              cell: (item) => (
                <div className='flex min-w-0 flex-col gap-1'>
                  <TruncatedCell
                    tabIndex={0}
                    tooltipContent={item.rule.user_agents.join('\n')}
                  >
                    <code>{item.rule.user_agents.join(' · ') || '—'}</code>
                  </TruncatedCell>
                  {item.rule.user_agents.length > 0 && (
                    <span className='text-muted-foreground text-xs'>
                      {item.rule.ua_match === 'exact'
                        ? t('Exact match')
                        : t('Fuzzy match')}
                    </span>
                  )}
                </div>
              ),
            },
            {
              id: 'logic',
              header: t('Condition logic'),
              className: 'w-32',
              cell: (item) => (
                <StatusBadge
                  label={item.rule.match_mode === 'all' ? t('AND') : t('OR')}
                  variant='neutral'
                  copyable={false}
                />
              ),
            },
            {
              id: 'actions',
              header: t('Actions'),
              className: 'w-24 text-right',
              cell: (item) => (
                <div className='flex items-center justify-end gap-0.5'>
                  <Tooltip>
                    <TooltipTrigger
                      render={
                        <Button
                          variant='ghost'
                          size='icon'
                          aria-label={t('Edit discount rule')}
                          onClick={() => openEditor(item.index)}
                        >
                          <Edit aria-hidden='true' />
                        </Button>
                      }
                    />
                    <TooltipContent>{t('Edit discount rule')}</TooltipContent>
                  </Tooltip>
                  <DataTableRowActionMenu ariaLabel={t('More actions')}>
                    <DropdownMenuGroup>
                      <DropdownMenuItem
                        variant='destructive'
                        onClick={() => setDeletingIndex(item.index)}
                      >
                        <Trash2 aria-hidden='true' />
                        {t('Delete Rule')}
                      </DropdownMenuItem>
                    </DropdownMenuGroup>
                  </DataTableRowActionMenu>
                </div>
              ),
            },
          ]}
        />
      )}

      <DiscountRuleEditorDialog
        open={editorOpen}
        onOpenChange={setEditorOpen}
        rule={editingIndex === null ? null : (rules[editingIndex] ?? null)}
        onSave={handleSaveRule}
      />

      <ConfirmDialog
        open={deletingIndex !== null}
        onOpenChange={(open) => {
          if (!open) setDeletingIndex(null)
        }}
        title={t('Delete Rule')}
        desc={t(
          'Delete discount rule "{{name}}"? Remember to save afterwards.',
          {
            name: deletingIndex === null ? '' : rules[deletingIndex]?.name,
          }
        )}
        destructive
        confirmText={t('Delete')}
        handleConfirm={() => {
          setRules(rules.filter((_, i) => i !== deletingIndex))
          setDeletingIndex(null)
        }}
      />
    </SettingsSection>
  )
}
