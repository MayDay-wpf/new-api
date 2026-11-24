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

import { COUPON_VALIDATION } from '../constants'
import type { CreateCouponPayload } from '../types'
import { parseUserIdList } from './format'

// ============================================================================
// Form Schema (use getCouponFormSchema(t) in components for i18n messages)
// ============================================================================

export function getCouponFormSchema(t: TFunction) {
  return z
    .object({
      name: z
        .string()
        .trim()
        .min(
          COUPON_VALIDATION.NAME_MIN_LENGTH,
          t('Coupon name length must be between 1-64')
        )
        .max(
          COUPON_VALIDATION.NAME_MAX_LENGTH,
          t('Coupon name length must be between 1-64')
        ),
      scope: z.enum(['topup', 'subscription', 'all']),
      threshold: z
        .number({ error: t('Coupon threshold cannot be negative') })
        .min(0, t('Coupon threshold cannot be negative')),
      discount: z
        .number({ error: t('Coupon discount must be greater than 0') })
        .gt(0, t('Coupon discount must be greater than 0')),
      stackable: z.boolean(),
      never_expires: z.boolean(),
      expires_at: z.date().nullable(),
      target_type: z.enum(['all', 'custom']),
      user_ids_text: z.string(),
      groups: z.array(z.string()),
    })
    .superRefine((data, ctx) => {
      if (!data.never_expires) {
        if (!data.expires_at || data.expires_at.getTime() <= Date.now()) {
          ctx.addIssue({
            code: 'custom',
            path: ['expires_at'],
            message: t('Coupon expiry time must be in the future'),
          })
        }
      }
      if (data.target_type === 'custom') {
        const parsed = parseUserIdList(data.user_ids_text)
        if (parsed.invalid.length > 0) {
          ctx.addIssue({
            code: 'custom',
            path: ['user_ids_text'],
            message: t('Invalid user IDs: {{ids}}', {
              ids: parsed.invalid.slice(0, 5).join(', '),
            }),
          })
        } else if (parsed.ids.length === 0 && data.groups.length === 0) {
          ctx.addIssue({
            code: 'custom',
            path: ['user_ids_text'],
            message: t('Select at least one recipient'),
          })
        }
      }
    })
}

export type CouponFormValues = z.infer<ReturnType<typeof getCouponFormSchema>>

export const COUPON_FORM_DEFAULT_VALUES: CouponFormValues = {
  name: '',
  scope: 'all',
  threshold: 0,
  discount: 1,
  stackable: false,
  never_expires: true,
  expires_at: null,
  target_type: 'all',
  user_ids_text: '',
  groups: [],
}

/** Round settlement amounts to the 2 decimal places the backend stores. */
function roundAmount(value: number): number {
  return Math.round(value * 100) / 100
}

export function transformCouponFormToPayload(
  data: CouponFormValues
): CreateCouponPayload {
  const isCustom = data.target_type === 'custom'
  return {
    name: data.name.trim(),
    scope: data.scope,
    threshold: roundAmount(data.threshold),
    discount: roundAmount(data.discount),
    stackable: data.stackable,
    expires_at:
      data.never_expires || !data.expires_at
        ? 0
        : Math.floor(data.expires_at.getTime() / 1000),
    target_type: data.target_type,
    user_ids: isCustom ? parseUserIdList(data.user_ids_text).ids : [],
    groups: isCustom ? data.groups : [],
  }
}
