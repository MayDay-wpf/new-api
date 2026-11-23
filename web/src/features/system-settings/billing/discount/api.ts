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

export type DiscountRule = {
  name: string
  enabled: boolean
  ratio: number
  user_ids: number[]
  channel_ids: number[]
  user_agents: string[]
  match_mode: DiscountMatchMode
  ua_match: DiscountUAMatch
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
    }))
  } catch {
    return []
  }
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
