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
import type { TFunction } from 'i18next'

import dayjs from '@/lib/dayjs'

import type {
  PlanQuotaLimit,
  QuotaLimitUsage,
  SubscriptionPlan,
} from '../types'

export function formatDuration(
  plan: Partial<SubscriptionPlan>,
  t: TFunction
): string {
  const unit = plan?.duration_unit || 'month'
  const value = plan?.duration_value || 1
  const unitLabels: Record<string, string> = {
    year: t('years'),
    month: t('months'),
    day: t('days'),
    hour: t('hours'),
    custom: t('Custom (seconds)'),
  }
  if (unit === 'custom') {
    const seconds = plan?.custom_seconds || 0
    if (seconds >= 86400) return `${Math.floor(seconds / 86400)} ${t('days')}`
    if (seconds >= 3600) return `${Math.floor(seconds / 3600)} ${t('hours')}`
    return `${seconds} ${t('seconds')}`
  }
  return `${value} ${unitLabels[unit] || unit}`
}

export function formatResetPeriod(
  plan: Partial<SubscriptionPlan>,
  t: TFunction
): string {
  const period = plan?.quota_reset_period || 'never'
  if (period === 'daily') return t('Daily')
  if (period === 'weekly') return t('Weekly')
  if (period === 'monthly') return t('Monthly')
  if (period === 'custom') {
    const seconds = Number(plan?.quota_reset_custom_seconds || 0)
    if (seconds >= 86400) return `${Math.floor(seconds / 86400)} ${t('days')}`
    if (seconds >= 3600) return `${Math.floor(seconds / 3600)} ${t('hours')}`
    if (seconds >= 60) return `${Math.floor(seconds / 60)} ${t('minutes')}`
    return `${seconds} ${t('seconds')}`
  }
  return t('No Reset')
}

export function formatTimestamp(ts: number): string {
  if (!ts) return '-'
  return dayjs(ts * 1000).format('YYYY-MM-DD HH:mm:ss')
}

// ============================================================================
// Independent quota windows
// ============================================================================

export function formatQuotaLimitWindow(
  limit: Partial<PlanQuotaLimit>,
  t: TFunction
): string {
  const period = limit?.period || 'daily'
  if (period === 'daily') return t('Daily')
  if (period === 'weekly') return t('Weekly')
  if (period === 'monthly') return t('Monthly')
  const seconds = Number(limit?.custom_seconds || 0)
  if (seconds >= 86400) return `${Math.floor(seconds / 86400)} ${t('days')}`
  if (seconds >= 3600) return `${Math.floor(seconds / 3600)} ${t('hours')}`
  if (seconds >= 60) return `${Math.floor(seconds / 60)} ${t('minutes')}`
  return `${seconds} ${t('seconds')}`
}

export function quotaLimitKey(limit: Partial<PlanQuotaLimit>): string {
  if (limit?.period === 'custom') {
    return `custom:${Number(limit.custom_seconds || 0)}`
  }
  return limit?.period || ''
}

// getQuotaLimitUsage returns the usage of the window containing now. Stored
// usage from a previous window is treated as zero until the backend refreshes
// it.
export function getQuotaLimitUsage(
  limit: Partial<PlanQuotaLimit>,
  usages: QuotaLimitUsage[] | undefined,
  nowSeconds: number = Date.now() / 1000
): number {
  const key = quotaLimitKey(limit)
  const usage = (usages || []).find((item) => item.key === key)
  if (!usage) return 0
  if (usage.window_end > 0 && usage.window_end <= nowSeconds) return 0
  return Number(usage.used || 0)
}

// getQuotaLimitResetTime returns the end of the current window, or 0 when the
// stored window already elapsed (the backend will roll it over on next use).
export function getQuotaLimitResetTime(
  limit: Partial<PlanQuotaLimit>,
  usages: QuotaLimitUsage[] | undefined,
  nowSeconds: number = Date.now() / 1000
): number {
  const key = quotaLimitKey(limit)
  const usage = (usages || []).find((item) => item.key === key)
  if (!usage || usage.window_end <= nowSeconds) return 0
  return Number(usage.window_end || 0)
}
