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
import { z } from 'zod'

import { parseQuotaFromDollars, quotaUnitsToEditableAmount } from '@/lib/format'

import {
  REDEMPTION_VALIDATION,
  getRedemptionFormErrorMessages,
} from '../constants'
import type { RedemptionFormData, Redemption, RedemptionType } from '../types'

// ============================================================================
// Form Schema (use getRedemptionFormSchema(t) in components for i18n messages)
// ============================================================================

export function getRedemptionFormSchema(t: TFunction) {
  const msg = getRedemptionFormErrorMessages(t)
  return z
    .object({
      type: z.enum(['quota', 'subscription', 'reset']),
      name: z
        .string()
        .min(REDEMPTION_VALIDATION.NAME_MIN_LENGTH, msg.NAME_LENGTH_INVALID)
        .max(REDEMPTION_VALIDATION.NAME_MAX_LENGTH, msg.NAME_LENGTH_INVALID),
      quota_dollars: z.number().min(0, t('Quota must be a positive number')),
      plan_id: z.number(),
      reset_plan_id: z.number(),
      reset_quota_limit_keys: z.array(z.string()),
      reset_advance_time: z.boolean(),
      expired_time: z.date().nullable(),
      max_uses: z
        .number()
        .int()
        .min(0, t('Usage limits cannot be negative')),
      per_user_limit: z
        .number()
        .int()
        .min(0, t('Usage limits cannot be negative')),
      count: z
        .number()
        .min(REDEMPTION_VALIDATION.COUNT_MIN, msg.COUNT_INVALID)
        .max(REDEMPTION_VALIDATION.COUNT_MAX, msg.COUNT_INVALID)
        .optional(),
    })
    .superRefine((data, ctx) => {
      if (data.type === 'subscription' && data.plan_id <= 0) {
        ctx.addIssue({
          code: 'custom',
          path: ['plan_id'],
          message: t('Please select a subscription plan'),
        })
      }
      if (data.type === 'reset') {
        if (data.reset_plan_id <= 0) {
          ctx.addIssue({
            code: 'custom',
            path: ['reset_plan_id'],
            message: t('Please select a subscription plan'),
          })
        }
        if (data.reset_quota_limit_keys.length === 0) {
          ctx.addIssue({
            code: 'custom',
            path: ['reset_quota_limit_keys'],
            message: t('Select at least one quota limit to reset'),
          })
        }
      }
    })
}

export type RedemptionFormValues = {
  type: RedemptionType
  name: string
  quota_dollars: number
  plan_id: number
  reset_plan_id: number
  reset_quota_limit_keys: string[]
  reset_advance_time: boolean
  expired_time: Date | null
  max_uses: number
  per_user_limit: number
  count?: number
}

// ============================================================================
// Form Defaults
// ============================================================================

export const REDEMPTION_FORM_DEFAULT_VALUES: RedemptionFormValues = {
  type: 'quota',
  name: '',
  quota_dollars: 10,
  plan_id: 0,
  reset_plan_id: 0,
  reset_quota_limit_keys: [],
  reset_advance_time: true,
  expired_time: null,
  max_uses: 1,
  per_user_limit: 1,
  count: 1,
}

// ============================================================================
// Form Data Transformation
// ============================================================================

/**
 * Transform form data to API payload
 */
export function transformFormDataToPayload(
  data: RedemptionFormValues
): RedemptionFormData {
  const isSubscription = data.type === 'subscription'
  const isReset = data.type === 'reset'
  return {
    name: data.name,
    quota: isSubscription || isReset ? 0 : parseQuotaFromDollars(data.quota_dollars),
    plan_id: isSubscription ? data.plan_id : 0,
    reset_plan_id: isReset ? data.reset_plan_id : 0,
    reset_quota_limit_keys: isReset ? data.reset_quota_limit_keys : [],
    reset_advance_time: isReset ? data.reset_advance_time : false,
    expired_time: data.expired_time
      ? Math.floor(data.expired_time.getTime() / 1000)
      : 0,
    max_uses: data.max_uses,
    per_user_limit: data.per_user_limit,
    count: data.count || 1,
  }
}

/**
 * Transform redemption data to form defaults
 */
export function transformRedemptionToFormDefaults(
  redemption: Redemption
): RedemptionFormValues {
  const planId = redemption.plan_id ?? 0
  const resetPlanId = redemption.reset_plan_id ?? 0
  let type: RedemptionType = 'quota'
  if (resetPlanId > 0) {
    type = 'reset'
  } else if (planId > 0) {
    type = 'subscription'
  }
  return {
    type,
    name: redemption.name,
    quota_dollars:
      type === 'quota'
        ? quotaUnitsToEditableAmount(redemption.quota)
        : REDEMPTION_FORM_DEFAULT_VALUES.quota_dollars,
    plan_id: planId,
    reset_plan_id: resetPlanId,
    reset_quota_limit_keys: redemption.reset_quota_limit_keys ?? [],
    reset_advance_time: redemption.reset_advance_time ?? true,
    expired_time:
      redemption.expired_time > 0
        ? new Date(redemption.expired_time * 1000)
        : null,
    max_uses: redemption.max_uses ?? 1,
    per_user_limit: redemption.per_user_limit ?? 1,
    count: 1,
  }
}
