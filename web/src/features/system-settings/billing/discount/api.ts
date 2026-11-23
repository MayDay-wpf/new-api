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
import { api } from '@/lib/api'

export type DiscountMatchMode = 'any' | 'all'
export type DiscountUAMatch = 'contains' | 'exact'
export type DiscountSchedulePeriod = 'daily' | 'weekly' | 'monthly'

/**
 * Repeating activation window of a discount rule. Every field may stay empty,
 * and a rule without a schedule applies unconditionally.
 */
export type DiscountSchedule = {
  timezone: string
  weekdays: number[]
  month_days: number[]
  start_time: string
  end_time: string
}

export type DiscountRule = {
  name: string
  enabled: boolean
  ratio: number
  user_ids: number[]
  channel_ids: number[]
  user_agents: string[]
  match_mode: DiscountMatchMode
  ua_match: DiscountUAMatch
  schedule: DiscountSchedule | null
}

export type RecentUserAgent = {
  user_agent: string
  count: number
}

export type DiscountChannel = {
  id: number
  name: string
  tag: string
}

export const DISCOUNT_RULES_OPTION_KEY = 'billing_discount_setting.rules'
export const MAX_DISCOUNT_RATIO = 10

export function parseDiscountRules(raw: string | undefined): DiscountRule[] {
  if (!raw) return []
  try {
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed)) return []
    return parsed.map((item: Partial<DiscountRule>) => ({
      name: item.name ?? '',
      enabled: item.enabled ?? false,
      ratio: Number(item.ratio) || 1,
      user_ids: Array.isArray(item.user_ids) ? item.user_ids : [],
      channel_ids: Array.isArray(item.channel_ids)
        ? item.channel_ids.filter(
            (id) => Number.isInteger(id) && Number(id) > 0
          )
        : [],
      user_agents: Array.isArray(item.user_agents) ? item.user_agents : [],
      match_mode: item.match_mode === 'all' ? 'all' : 'any',
      ua_match: item.ua_match === 'exact' ? 'exact' : 'contains',
      schedule: parseDiscountSchedule(item.schedule),
    }))
  } catch {
    return []
  }
}

function normalizeDayList(value: unknown, min: number, max: number): number[] {
  if (!Array.isArray(value)) return []
  const days = value.filter(
    (day): day is number =>
      Number.isInteger(day) && day >= min && day <= max
  )
  return [...new Set(days)].sort((a, b) => a - b)
}

function parseDiscountSchedule(raw: unknown): DiscountSchedule | null {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null
  const item = raw as Partial<DiscountSchedule>
  const schedule: DiscountSchedule = {
    timezone: typeof item.timezone === 'string' ? item.timezone.trim() : '',
    weekdays: normalizeDayList(item.weekdays, 0, 6),
    month_days: normalizeDayList(item.month_days, 1, 31),
    start_time: typeof item.start_time === 'string' ? item.start_time : '',
    end_time: typeof item.end_time === 'string' ? item.end_time : '',
  }
  return isDiscountScheduleEmpty(schedule) ? null : schedule
}

/** A schedule containing nothing but blank values restricts nothing. */
export function isDiscountScheduleEmpty(
  schedule: DiscountSchedule | null | undefined
): boolean {
  if (!schedule) return true
  return (
    !schedule.timezone?.trim() &&
    schedule.weekdays.length === 0 &&
    schedule.month_days.length === 0 &&
    !schedule.start_time?.trim() &&
    !schedule.end_time?.trim()
  )
}

export function getSchedulePeriod(
  schedule: DiscountSchedule | null
): DiscountSchedulePeriod {
  if (schedule && schedule.month_days.length > 0) return 'monthly'
  if (schedule && schedule.weekdays.length > 0) return 'weekly'
  return 'daily'
}

export async function getRecentUserAgents(): Promise<{
  success: boolean
  message?: string
  data?: RecentUserAgent[]
}> {
  const res = await api.get('/api/option/billing_discount_user_agents')
  return res.data
}

export async function getDiscountChannels(): Promise<{
  success: boolean
  message?: string
  data?: DiscountChannel[]
}> {
  const res = await api.get('/api/option/billing_discount_channels')
  return res.data
}
