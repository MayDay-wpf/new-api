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
import { z } from 'zod'

// ============================================================================
// Redemption Schema & Types
// ============================================================================

export const redemptionSchema = z.object({
  id: z.number(),
  user_id: z.number(),
  name: z.string(),
  key: z.string(),
  status: z.number(), // 1: enabled, 2: disabled, 3: used
  quota: z.number(),
  created_time: z.number(),
  redeemed_time: z.number(),
  expired_time: z.number(), // 0 for never expires
  used_user_id: z.number(),
  used_count: z.number().optional(), // times the code has been used
  max_uses: z.number().optional(), // total usage limit, 0 = unlimited
  per_user_limit: z.number().optional(), // per-account usage limit, 0 = unlimited
  plan_id: z.number().optional(), // > 0: subscription code granting this plan
  reset_plan_id: z.number().nullish(), // > 0: reset card for this plan
  reset_quota_limit_keys: z.array(z.string()).nullish(), // reset card target windows
  reset_advance_time: z.boolean().nullish(), // reset card advances next reset time
})

export type Redemption = z.infer<typeof redemptionSchema>

export type RedemptionType = 'quota' | 'subscription' | 'reset'

// ============================================================================
// API Request/Response Types
// ============================================================================

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface GetRedemptionsParams {
  p?: number
  page_size?: number
}

export interface GetRedemptionsResponse {
  success: boolean
  message?: string
  data?: {
    items: Redemption[]
    total: number
    page: number
    page_size: number
  }
}

export interface SearchRedemptionsParams {
  keyword?: string
  status?: string
  p?: number
  page_size?: number
}

export interface RedemptionFormData {
  id?: number
  name: string
  quota: number
  plan_id: number // 0 for balance codes
  reset_plan_id?: number // > 0 for reset cards
  reset_quota_limit_keys?: string[]
  reset_advance_time?: boolean
  expired_time: number
  max_uses: number // 0 = unlimited total uses
  per_user_limit: number // 0 = unlimited uses per account
  count?: number // Only for create
  status?: number // Only for status update
}

// ============================================================================
// Dialog Types
// ============================================================================

export type RedemptionsDialogType = 'create' | 'update' | 'delete' | 'view'
