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
import { useEffect, useRef, useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Progress } from '@/components/ui/progress'
import {
  formatQuotaLimitWindow,
  getQuotaLimitUsage,
  quotaLimitKey,
} from '@/features/subscriptions/lib/format'
import { toIntlLocale } from '@/i18n/languages'
import {
  formatQuota,
  formatTimestampRelative,
  formatTimestampToDate,
} from '@/lib/format'

import type { AdminUserSubscriptionRow } from '../types'

const EXPIRING_SOON_WINDOW = 7 * 24 * 60 * 60

function subscriptionIsActive(row: AdminUserSubscriptionRow, now: number) {
  const sub = row.subscription
  return sub.status === 'active' && sub.end_time > now
}

export function SubscriptionUserCell(props: { row: AdminUserSubscriptionRow }) {
  const { t } = useTranslation()
  const sub = props.row.subscription
  const primary = props.row.display_name || props.row.username
  return (
    <div className='max-w-[220px] min-w-0'>
      <div className='truncate font-medium'>{primary || `#${sub.user_id}`}</div>
      <div className='text-muted-foreground truncate text-xs'>
        {props.row.username ? `@${props.row.username}` : t('Deleted user')}
        {props.row.email ? ` · ${props.row.email}` : ''}
      </div>
    </div>
  )
}

export function SubscriptionPlanCell(props: { row: AdminUserSubscriptionRow }) {
  const sub = props.row.subscription
  return (
    <div className='max-w-[200px] min-w-0'>
      <div className='truncate'>
        {props.row.plan_title || `#${sub.plan_id}`}
      </div>
      {props.row.plan_subtitle ? (
        <div className='text-muted-foreground truncate text-xs'>
          {props.row.plan_subtitle}
        </div>
      ) : null}
    </div>
  )
}

export function SubscriptionStatusCell(props: {
  row: AdminUserSubscriptionRow
}) {
  const { t } = useTranslation()
  // eslint-disable-next-line react-hooks/purity
  const now = Math.floor(Date.now() / 1000)
  const sub = props.row.subscription
  if (sub.status === 'cancelled') {
    return (
      <StatusBadge label={t('Cancelled')} variant='neutral' copyable={false} />
    )
  }
  if (subscriptionIsActive(props.row, now)) {
    return (
      <StatusBadge label={t('Active')} variant='success' copyable={false} />
    )
  }
  return <StatusBadge label={t('Expired')} variant='neutral' copyable={false} />
}

export function SubscriptionUsageCell(props: {
  row: AdminUserSubscriptionRow
}) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const closeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const cancelClose = () => {
    if (closeTimerRef.current) {
      clearTimeout(closeTimerRef.current)
      closeTimerRef.current = null
    }
  }

  const openPopover = () => {
    cancelClose()
    setOpen(true)
  }

  const scheduleClose = () => {
    cancelClose()
    closeTimerRef.current = setTimeout(() => setOpen(false), 120)
  }

  useEffect(() => {
    return () => {
      if (closeTimerRef.current) {
        clearTimeout(closeTimerRef.current)
      }
    }
  }, [])

  const sub = props.row.subscription
  // eslint-disable-next-line react-hooks/purity
  const now = Math.floor(Date.now() / 1000)
  const total = sub.amount_total || 0
  const used = sub.amount_used || 0
  const hasTotal = total > 0
  const remaining = Math.max(total - used, 0)
  const percent = hasTotal ? Math.min(Math.round((used / total) * 100), 100) : 0
  const quotaWindows = (props.row.plan_quota_limits ?? []).map((limit) => {
    const amount = Number(limit.amount || 0)
    const windowUsed = getQuotaLimitUsage(limit, sub.quota_limit_usage, now)
    return {
      key: quotaLimitKey(limit),
      label: formatQuotaLimitWindow(limit, t),
      amount,
      used: windowUsed,
      percent:
        amount > 0 ? Math.min(Math.round((windowUsed / amount) * 100), 100) : 0,
    }
  })
  const primaryWindow = quotaWindows[0]

  let triggerContent: ReactNode
  if (hasTotal) {
    triggerContent = (
      <>
        <div className='flex items-center justify-between gap-3 text-xs tabular-nums'>
          <span className='text-muted-foreground'>
            {t('Used')} {formatQuota(used)}
          </span>
          <span className='text-muted-foreground'>{percent}%</span>
        </div>
        <div className='bg-muted h-1.5 w-full overflow-hidden rounded-full'>
          <div
            className='bg-primary h-full rounded-full'
            style={{ width: `${percent}%` }}
          />
        </div>
        <div className='text-xs tabular-nums'>
          <span className='text-muted-foreground'>{t('Remaining')}</span>{' '}
          <span className='font-medium'>{formatQuota(remaining)}</span>
        </div>
      </>
    )
  } else if (primaryWindow) {
    triggerContent = (
      <>
        <div className='flex items-center justify-between gap-3 text-xs tabular-nums'>
          <span className='text-muted-foreground'>{primaryWindow.label}</span>
          <span className='text-muted-foreground'>
            {primaryWindow.percent}%
          </span>
        </div>
        <div className='bg-muted h-1.5 w-full overflow-hidden rounded-full'>
          <div
            className='bg-primary h-full rounded-full'
            style={{ width: `${primaryWindow.percent}%` }}
          />
        </div>
        <div className='text-xs tabular-nums'>
          <span className='font-medium'>{formatQuota(primaryWindow.used)}</span>
          <span className='text-muted-foreground'>
            {' / '}
            {primaryWindow.amount > 0
              ? formatQuota(primaryWindow.amount)
              : t('Unlimited')}
          </span>
        </div>
      </>
    )
  } else {
    triggerContent = (
      <span className='text-muted-foreground'>{t('Unlimited')}</span>
    )
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        nativeButton={false}
        render={
          <div
            className='min-w-[160px] cursor-help space-y-1'
            onMouseEnter={openPopover}
            onMouseLeave={scheduleClose}
            onFocus={openPopover}
            onBlur={scheduleClose}
          >
            {triggerContent}
          </div>
        }
      />
      <PopoverContent
        align='start'
        className='max-h-[calc(100vh-2rem)] max-w-[calc(100vw-2rem)] overflow-y-auto'
        onMouseEnter={openPopover}
        onMouseLeave={scheduleClose}
      >
        <div className='flex items-center justify-between gap-3'>
          <span className='font-medium'>{t('Usage')}</span>
          {hasTotal ? (
            <span className='text-muted-foreground text-xs tabular-nums'>
              {percent}%
            </span>
          ) : null}
        </div>
        {hasTotal ? (
          <>
            <Progress
              value={percent}
              aria-label={t('Usage')}
              className='[&_[data-slot=progress-track]]:h-2'
            />
            <dl className='grid grid-cols-[1fr_auto] gap-x-4 gap-y-1.5 text-sm tabular-nums'>
              <dt className='text-muted-foreground'>{t('Used amount')}</dt>
              <dd className='text-right'>{formatQuota(used)}</dd>
              <dt className='text-muted-foreground'>{t('Remaining')}</dt>
              <dd className='text-right font-medium'>
                {formatQuota(remaining)}
              </dd>
              <dt className='text-muted-foreground'>{t('Total')}</dt>
              <dd className='text-right'>{formatQuota(total)}</dd>
            </dl>
          </>
        ) : null}
        {quotaWindows.length > 0 ? (
          <div className={hasTotal ? 'space-y-3 border-t pt-3' : 'space-y-3'}>
            <div className='text-muted-foreground text-xs font-medium'>
              {t('Quota Limits')}
            </div>
            {quotaWindows.map((item) => (
              <div key={item.key} className='space-y-1'>
                <div className='flex items-center justify-between gap-3 text-xs tabular-nums'>
                  <span className='text-muted-foreground'>{item.label}</span>
                  <span className='font-medium'>
                    {formatQuota(item.used)}
                    {' / '}
                    {item.amount > 0
                      ? formatQuota(item.amount)
                      : t('Unlimited')}
                  </span>
                </div>
                <Progress
                  value={item.percent}
                  aria-label={item.label}
                  className='[&_[data-slot=progress-track]]:h-2'
                />
                {item.amount > 0 ? (
                  <div className='text-muted-foreground flex items-center justify-between gap-3 text-xs tabular-nums'>
                    <span>{item.percent}%</span>
                    <span>
                      {t('Remaining')}{' '}
                      {formatQuota(Math.max(item.amount - item.used, 0))}
                    </span>
                  </div>
                ) : null}
              </div>
            ))}
          </div>
        ) : null}
        {!hasTotal && !primaryWindow ? (
          <p className='text-muted-foreground text-sm'>{t('Unlimited')}</p>
        ) : null}
      </PopoverContent>
    </Popover>
  )
}

export function SubscriptionExpiryCell(props: {
  row: AdminUserSubscriptionRow
}) {
  const { i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  // eslint-disable-next-line react-hooks/purity
  const now = Math.floor(Date.now() / 1000)
  const sub = props.row.subscription
  const active = subscriptionIsActive(props.row, now)
  const expiringSoon =
    active && sub.end_time - now <= EXPIRING_SOON_WINDOW && sub.end_time > now

  return (
    <div className='min-w-[160px]'>
      <div className='font-mono text-sm'>
        {formatTimestampToDate(sub.end_time)}
      </div>
      {active ? (
        <div
          className={
            expiringSoon
              ? 'text-destructive text-xs'
              : 'text-muted-foreground text-xs'
          }
        >
          {formatTimestampRelative(sub.end_time, 'seconds', locale)}
        </div>
      ) : null}
    </div>
  )
}
