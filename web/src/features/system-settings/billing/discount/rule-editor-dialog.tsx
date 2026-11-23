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
import { searchUsers } from '@/features/users/api'
import { useDebounce } from '@/hooks/use-debounce'

import { SettingsSwitchField } from '../../components/settings-form-layout'
import {
  getDiscountChannels,
  getRecentUserAgents,
  MAX_DISCOUNT_RATIO,
  type DiscountMatchMode,
  type DiscountRule,
  type DiscountUAMatch,
} from './api'

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
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  rule: DiscountRule | null
  onSave: (rule: DiscountRule) => void
}

export function DiscountRuleEditorDialog(props: Props) {
  const { t } = useTranslation()
  const [draft, setDraft] = useState<DiscountRule>(EMPTY_RULE)
  const [ratioText, setRatioText] = useState('0.5')
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
    setUserKeyword('')
    setError('')
  }, [props.open, props.rule])

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
    props.onSave({ ...draft, name, ratio, user_agents: userAgents })
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
