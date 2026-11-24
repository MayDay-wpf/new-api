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

import { formatCurrency } from '@/features/wallet/lib/format'

import { COUPON_SCOPE_LABELS } from '../constants'
import type { CouponScope } from '../types'

/** Settlement amounts (threshold, discount) use the payment amount format. */
export function formatCouponAmount(amount: number): string {
  return formatCurrency(amount)
}

/** "Spend X, save Y" or "No minimum, save Y". */
export function getCouponRuleLabel(
  coupon: { threshold: number; discount: number },
  t: TFunction
): string {
  const discount = formatCouponAmount(coupon.discount)
  if (coupon.threshold > 0) {
    return t('Spend {{threshold}}, save {{discount}}', {
      threshold: formatCouponAmount(coupon.threshold),
      discount,
    })
  }
  return t('No minimum, save {{discount}}', { discount })
}

export function getCouponScopeLabel(scope: CouponScope, t: TFunction): string {
  return t(COUPON_SCOPE_LABELS[scope] ?? scope)
}

/** Expiry timestamps are Unix seconds; 0 never expires. */
export function isCouponExpired(
  expiresAt: number,
  nowSeconds: number = Date.now() / 1000
): boolean {
  return expiresAt > 0 && expiresAt <= nowSeconds
}

/**
 * Parse user IDs typed as a list separated by commas, spaces or new lines.
 * Returns the unique positive integers and the tokens that are not valid IDs.
 */
export function parseUserIdList(input: string): {
  ids: number[]
  invalid: string[]
} {
  const ids: number[] = []
  const invalid: string[] = []
  const seen = new Set<number>()
  for (const token of input.split(/[\s,，]+/)) {
    if (!token) continue
    const id = /^\d+$/.test(token) ? Number(token) : Number.NaN
    if (!Number.isSafeInteger(id) || id <= 0) {
      invalid.push(token)
      continue
    }
    if (!seen.has(id)) {
      seen.add(id)
      ids.push(id)
    }
  }
  return { ids, invalid }
}
