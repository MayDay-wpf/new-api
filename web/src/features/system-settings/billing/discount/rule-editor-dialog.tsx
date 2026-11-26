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
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { MultiSelect, type Option } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { COMMON_TIMEZONES } from '@/features/pricing/lib/billing-expr'
import { searchUsers } from '@/features/users/api'
import { useDebounce } from '@/hooks/use-debounce'
import { toIntlLocale } from '@/i18n/languages'

import { SettingsSwitchField } from '../../components/settings-form-layout'
import {
  getDiscountChannels,
  getRecentUserAgents,
  getSchedulePeriod,
  isDiscountScheduleEmpty,
  MAX_DISCOUNT_RATIO,
  type DiscountMatchMode,
  type DiscountRule,
  type DiscountSchedule,
  type DiscountSchedulePeriod,
  type DiscountUAMatch,
} from './api'
import { getWeekdayLabels } from './weekday-labels'

const RULE_FORM_ID = 'billing-discount-rule-form'

const EMPTY_RULE: DiscountRule = {
  name: '',
  enabled: true,
  ratio: 0.5,
  user_ids: [],
  channel_ids: [],
  user_agents: [],
  match_mode: 'any',
  ua_match: 'contains',
  schedule: null,
}

type ScheduleForm = {
  enabled: boolean
  period: DiscountSchedulePeriod
  weekdays: number[]
  monthDays: number[]
  startTime: string
  endTime: string
  timezone: string
}

const EMPTY_SCHEDULE_FORM: ScheduleForm = {
  enabled: false,
  period: 'daily',
  weekdays: [],
  monthDays: [],
  startTime: '',
  endTime: '',
  timezone: '',
}

function toScheduleForm(schedule: DiscountSchedule | null): ScheduleForm {
  if (!schedule || isDiscountScheduleEmpty(schedule)) {
    return EMPTY_SCHEDULE_FORM
  }
  return {
    enabled: true,
    period: getSchedulePeriod(schedule),
    weekdays: schedule.weekdays,
    monthDays: schedule.month_days,
    startTime: schedule.start_time,
    endTime: schedule.end_time,
    timezone: schedule.timezone,
  }
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  rule: DiscountRule | null
  onSave: (rule: DiscountRule) => void
}

export function DiscountRuleEditorDialog(props: Props) {
  const { t, i18n } = useTranslation()
  const [draft, setDraft] = useState<DiscountRule>(EMPTY_RULE)
  const [ratioText, setRatioText] = useState('0.5')
  const [scheduleForm, setScheduleForm] =
    useState<ScheduleForm>(EMPTY_SCHEDULE_FORM)
  const [userKeyword, setUserKeyword] = useState('')
  const [userLabels, setUserLabels] = useState<Map<string, string>>(
    () => new Map()
  )
  const [error, setError] = useState('')
  const debouncedKeyword = useDebounce(userKeyword.trim(), 300)

  useEffect(() => {
    if (!props.open) return
    const next = props.rule ?? EMPTY_RULE
    setDraft(next)
    setRatioText(String(next.ratio))
    setScheduleForm(toScheduleForm(next.schedule))
    setUserKeyword('')
    setError('')
  }, [props.open, props.rule])

  const weekdayLabels = useMemo(
    () =>
      getWeekdayLabels(toIntlLocale(i18n.resolvedLanguage || i18n.language)),
    [i18n.language, i18n.resolvedLanguage]
  )

  const timezoneOptions = useMemo(() => {
    const options = COMMON_TIMEZONES.map((zone) => ({
      value: zone.value,
      label: zone.label,
    }))
    const current = scheduleForm.timezone.trim()
    if (current && !options.some((option) => option.value === current)) {
      options.push({ value: current, label: current })
    }
    return options
  }, [scheduleForm.timezone])

  const usersQuery = useQuery({
    queryKey: ['billing-discount-users', debouncedKeyword],
    queryFn: () => searchUsers({ keyword: debouncedKeyword, page_size: 100 }),
    enabled: props.open,
  })

  const userAgentsQuery = useQuery({
    queryKey: ['billing-discount-user-agents'],
    queryFn: getRecentUserAgents,
    enabled: props.open,
  })

  const channelsQuery = useQuery({
    queryKey: ['billing-discount-channels'],
    queryFn: getDiscountChannels,
    enabled: props.open,
  })

  // Remember every username we have seen so selected chips keep a readable
  // label after the search keyword changes.
  useEffect(() => {
    const items = usersQuery.data?.data?.items
    if (!items?.length) return
    setUserLabels((prev) => {
      const next = new Map(prev)
      for (const user of items) {
        next.set(String(user.id), `${user.username} (#${user.id})`)
      }
      return next
    })
  }, [usersQuery.data])

  const userOptions = useMemo<Option[]>(() => {
    const items = usersQuery.data?.data?.items ?? []
    const options: Option[] = items.map((user) => ({
      value: String(user.id),
      label: `${user.username} (#${user.id})`,
      hint: user.display_name || undefined,
    }))
    for (const id of draft.user_ids) {
      const value = String(id)
      if (options.some((option) => option.value === value)) continue
      options.push({ value, label: userLabels.get(value) ?? `#${value}` })
    }
    return options
  }, [usersQuery.data, draft.user_ids, userLabels])

  const userAgentOptions = useMemo<Option[]>(
    () =>
      (userAgentsQuery.data?.data ?? []).map((item) => ({
        value: item.user_agent,
        label: item.user_agent,
        hint: t('{{count}} requests', { count: item.count }),
      })),
    [userAgentsQuery.data, t]
  )

  const channelOptions = useMemo<Option[]>(() => {
    const items = channelsQuery.data?.data ?? []
    const options: Option[] = items.map((channel) => ({
      value: String(channel.id),
      label: `${channel.name} (#${channel.id})`,
      hint: channel.tag || undefined,
    }))
    for (const id of draft.channel_ids) {
      const value = String(id)
      if (options.some((option) => option.value === value)) continue
      options.push({ value, label: `#${value}` })
    }
    return options
  }, [channelsQuery.data, draft.channel_ids])

  function handleSave(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const name = draft.name.trim()
    if (!name) {
      setError(t('Rule name is required'))
      return
    }
    const ratio = Number(ratioText)
    if (!Number.isFinite(ratio) || ratio <= 0 || ratio > MAX_DISCOUNT_RATIO) {
      setError(
        t('Ratio must be greater than 0 and at most {{max}}', {
          max: MAX_DISCOUNT_RATIO,
        })
      )
      return
    }
    const userAgents = draft.user_agents
      .map((value) => value.trim())
      .filter(Boolean)
    if (
      draft.user_ids.length === 0 &&
      draft.channel_ids.length === 0 &&
      userAgents.length === 0
    ) {
      setError(t('Select at least one user, channel or User-Agent'))
      return
    }
    let schedule: DiscountSchedule | null = null
    if (scheduleForm.enabled) {
      const startTime = scheduleForm.startTime
      const endTime = scheduleForm.endTime
      if (Boolean(startTime) !== Boolean(endTime)) {
        setError(t('Start time and end time are required'))
        return
      }
      if (startTime && startTime === endTime) {
        setError(t('Start time and end time must differ'))
        return
      }
      if (scheduleForm.period === 'daily' && !startTime) {
        setError(t('Fill in a time range or turn off the time limit'))
        return
      }
      const weekdays =
        scheduleForm.period === 'weekly'
          ? [...scheduleForm.weekdays].sort((a, b) => a - b)
          : []
      const monthDays =
        scheduleForm.period === 'monthly'
          ? [...scheduleForm.monthDays].sort((a, b) => a - b)
          : []
      if (scheduleForm.period === 'weekly' && weekdays.length === 0) {
        setError(t('Select at least one weekday'))
        return
      }
      if (scheduleForm.period === 'monthly' && monthDays.length === 0) {
        setError(t('Select at least one day of month'))
        return
      }
      schedule = {
        timezone: scheduleForm.timezone.trim(),
        weekdays,
        month_days: monthDays,
        start_time: startTime,
        end_time: endTime,
      }
    }
    props.onSave({
      ...draft,
      name,
      ratio,
      user_agents: userAgents,
      schedule,
    })
    props.onOpenChange(false)
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={props.rule ? t('Edit discount rule') : t('Add discount rule')}
      contentClassName='sm:max-w-2xl'
      contentHeight='auto'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => props.onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='submit' form={RULE_FORM_ID}>
            {t('Save')}
          </Button>
        </>
      }
    >
      <form
        id={RULE_FORM_ID}
        onSubmit={handleSave}
        className='min-w-0 space-y-4'
        autoComplete='off'
      >
        <div className='grid gap-3 sm:grid-cols-2'>
          <div className='grid gap-1.5'>
            <Label required htmlFor='discount-rule-name'>
              {t('Name')}
            </Label>
            <Input
              id='discount-rule-name'
              value={draft.name}
              placeholder={t('e.g. Partner discount')}
              onChange={(event) =>
                setDraft({ ...draft, name: event.target.value })
              }
            />
          </div>
          <div className='grid gap-1.5'>
            <Label required htmlFor='discount-rule-ratio'>
              {t('Discount ratio')}
            </Label>
            <Input
              id='discount-rule-ratio'
              type='number'
              step='0.01'
              min={0}
              max={MAX_DISCOUNT_RATIO}
              value={ratioText}
              aria-describedby='discount-rule-ratio-description'
              onChange={(event) => setRatioText(event.target.value)}
            />
            <p
              id='discount-rule-ratio-description'
              className='text-muted-foreground text-xs'
            >
              {t(
                'Final charge is multiplied by this ratio, e.g. 0.5 means half price'
              )}
            </p>
          </div>
        </div>

        <SettingsSwitchField
          controlId='discount-rule-enabled'
          checked={draft.enabled}
          onCheckedChange={(enabled) => setDraft({ ...draft, enabled })}
          label={t('Enabled')}
          className='py-0'
        />

        <div className='space-y-3 rounded-xl border px-3 py-2.5'>
          <SettingsSwitchField
            controlId='discount-rule-schedule-enabled'
            checked={scheduleForm.enabled}
            onCheckedChange={(enabled) =>
              setScheduleForm({ ...scheduleForm, enabled })
            }
            label={t('Limit to a time window')}
            description={t(
              'Off: the rule applies immediately and always. On: it only applies inside the repeating window below.'
            )}
            className='py-0'
          />

          {scheduleForm.enabled && (
            <div className='space-y-3'>
              <div className='grid gap-3 sm:grid-cols-2'>
                <div className='grid gap-1.5'>
                  <Label htmlFor='discount-rule-schedule-period'>
                    {t('Repeat')}
                  </Label>
                  <NativeSelect
                    id='discount-rule-schedule-period'
                    className='w-full'
                    value={scheduleForm.period}
                    onChange={(event) =>
                      setScheduleForm({
                        ...scheduleForm,
                        period: event.target.value as DiscountSchedulePeriod,
                      })
                    }
                  >
                    <NativeSelectOption value='daily'>
                      {t('Daily')}
                    </NativeSelectOption>
                    <NativeSelectOption value='weekly'>
                      {t('Weekly')}
                    </NativeSelectOption>
                    <NativeSelectOption value='monthly'>
                      {t('Monthly')}
                    </NativeSelectOption>
                  </NativeSelect>
                </div>
                <div className='grid gap-1.5'>
                  <Label htmlFor='discount-rule-schedule-timezone'>
                    {t('Timezone')}
                  </Label>
                  <NativeSelect
                    id='discount-rule-schedule-timezone'
                    className='w-full'
                    value={scheduleForm.timezone}
                    onChange={(event) =>
                      setScheduleForm({
                        ...scheduleForm,
                        timezone: event.target.value,
                      })
                    }
                  >
                    <NativeSelectOption value=''>
                      {t('Server time')}
                    </NativeSelectOption>
                    {timezoneOptions.map((zone) => (
                      <NativeSelectOption key={zone.value} value={zone.value}>
                        {zone.label}
                      </NativeSelectOption>
                    ))}
                  </NativeSelect>
                </div>
              </div>

              {scheduleForm.period === 'weekly' && (
                <div className='grid gap-1.5'>
                  <Label>{t('Weekdays')}</Label>
                  <ToggleGroup
                    multiple
                    variant='outline'
                    size='sm'
                    spacing={1}
                    className='flex-wrap'
                    aria-label={t('Weekdays')}
                    value={scheduleForm.weekdays.map(String)}
                    onValueChange={(values) =>
                      setScheduleForm({
                        ...scheduleForm,
                        weekdays: values.map(Number).sort((a, b) => a - b),
                      })
                    }
                  >
                    {weekdayLabels.map((label, day) => (
                      <ToggleGroupItem
                        key={day}
                        value={String(day)}
                        className='aria-pressed:border-primary aria-pressed:bg-primary aria-pressed:text-primary-foreground min-w-9 px-2'
                      >
                        {label}
                      </ToggleGroupItem>
                    ))}
                  </ToggleGroup>
                </div>
              )}

              {scheduleForm.period === 'monthly' && (
                <div className='grid gap-1.5'>
                  <Label>{t('Days of month')}</Label>
                  <ToggleGroup
                    multiple
                    variant='outline'
                    size='sm'
                    spacing={1}
                    className='flex-wrap'
                    aria-label={t('Days of month')}
                    value={scheduleForm.monthDays.map(String)}
                    onValueChange={(values) =>
                      setScheduleForm({
                        ...scheduleForm,
                        monthDays: values.map(Number).sort((a, b) => a - b),
                      })
                    }
                  >
                    {Array.from({ length: 31 }, (_, index) => index + 1).map(
                      (day) => (
                        <ToggleGroupItem
                          key={day}
                          value={String(day)}
                          className='aria-pressed:border-primary aria-pressed:bg-primary aria-pressed:text-primary-foreground min-w-8 px-0 tabular-nums'
                        >
                          {day}
                        </ToggleGroupItem>
                      )
                    )}
                  </ToggleGroup>
                </div>
              )}

              <div className='grid gap-3 sm:grid-cols-2'>
                <div className='grid gap-1.5'>
                  <Label htmlFor='discount-rule-schedule-start'>
                    {t('Start time')}
                  </Label>
                  <Input
                    id='discount-rule-schedule-start'
                    type='time'
                    step={60}
                    value={scheduleForm.startTime}
                    onChange={(event) =>
                      setScheduleForm({
                        ...scheduleForm,
                        startTime: event.target.value,
                      })
                    }
                  />
                </div>
                <div className='grid gap-1.5'>
                  <Label htmlFor='discount-rule-schedule-end'>
                    {t('End time')}
                  </Label>
                  <Input
                    id='discount-rule-schedule-end'
                    type='time'
                    step={60}
                    value={scheduleForm.endTime}
                    onChange={(event) =>
                      setScheduleForm({
                        ...scheduleForm,
                        endTime: event.target.value,
                      })
                    }
                  />
                </div>
              </div>
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Leave both times empty for an all-day window. End time is exclusive, so 22:00–06:00 crosses midnight and still uses the selected day.'
                )}
              </p>
            </div>
          )}
        </div>

        <div className='grid gap-1.5'>
          <Label htmlFor='discount-rule-users'>{t('Users')}</Label>
          <Input
            value={userKeyword}
            placeholder={t('Search users by ID, username or email')}
            aria-label={t('Search users by ID, username or email')}
            onChange={(event) => setUserKeyword(event.target.value)}
          />
          <MultiSelect
            id='discount-rule-users'
            options={userOptions}
            selected={draft.user_ids.map(String)}
            onChange={(values) =>
              setDraft({
                ...draft,
                user_ids: values
                  .map(Number)
                  .filter((id) => Number.isInteger(id) && id > 0),
              })
            }
            placeholder={t('Select users')}
            maxVisibleChips={12}
          />
        </div>

        <div className='grid gap-1.5'>
          <Label htmlFor='discount-rule-channels'>{t('Channels')}</Label>
          <MultiSelect
            id='discount-rule-channels'
            options={channelOptions}
            selected={draft.channel_ids.map(String)}
            onChange={(values) =>
              setDraft({
                ...draft,
                channel_ids: values
                  .map(Number)
                  .filter((id) => Number.isInteger(id) && id > 0),
              })
            }
            placeholder={t('Select channels')}
            maxVisibleChips={8}
          />
        </div>

        <div className='grid gap-1.5'>
          <Label htmlFor='discount-rule-user-agents'>{t('User-Agent')}</Label>
          <MultiSelect
            id='discount-rule-user-agents'
            options={userAgentOptions}
            selected={draft.user_agents}
            onChange={(values) => setDraft({ ...draft, user_agents: values })}
            placeholder={t('Pick from recent request logs or type to add')}
            allowCreate
            maxVisibleChips={8}
          />
        </div>

        <div className='grid gap-3 sm:grid-cols-2'>
          <div className='grid gap-1.5'>
            <Label htmlFor='discount-rule-ua-match'>
              {t('User-Agent match')}
            </Label>
            <NativeSelect
              id='discount-rule-ua-match'
              className='w-full'
              value={draft.ua_match}
              onChange={(event) =>
                setDraft({
                  ...draft,
                  ua_match: event.target.value as DiscountUAMatch,
                })
              }
            >
              <NativeSelectOption value='contains'>
                {t('Fuzzy (contains, case-insensitive)')}
              </NativeSelectOption>
              <NativeSelectOption value='exact'>
                {t('Exact (whole value, case-insensitive)')}
              </NativeSelectOption>
            </NativeSelect>
          </div>
          <div className='grid gap-1.5'>
            <Label htmlFor='discount-rule-match-mode'>
              {t('Condition logic')}
            </Label>
            <NativeSelect
              id='discount-rule-match-mode'
              className='w-full'
              value={draft.match_mode}
              onChange={(event) =>
                setDraft({
                  ...draft,
                  match_mode: event.target.value as DiscountMatchMode,
                })
              }
            >
              <NativeSelectOption value='any'>
                {t('Any condition matches')}
              </NativeSelectOption>
              <NativeSelectOption value='all'>
                {t('All conditions match')}
              </NativeSelectOption>
            </NativeSelect>
            {draft.channel_ids.length > 0 && (
              <p className='text-muted-foreground text-xs'>
                {t(
                  'Channels always restrict the rule: requests served by another channel never match. The condition logic only combines users and User-Agents.'
                )}
              </p>
            )}
          </div>
        </div>

        {error && (
          <p role='alert' className='text-destructive text-sm'>
            {error}
          </p>
        )}
      </form>
    </Dialog>
  )
}
