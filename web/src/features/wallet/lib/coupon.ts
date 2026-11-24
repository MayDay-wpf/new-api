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
import type { UserCoupon } from '@/features/coupons/types'

// ============================================================================
// Coupon Selection Rules
// ----------------------------------------------------------------------------
// Mirrors the backend checkout validation so the UI can preview the final
// amount and disable coupons that would be rejected. The backend remains the
// source of truth.
// ============================================================================

/** Maximum number of coupons accepted by one payment request. */
export const MAX_COUPONS_PER_ORDER = 10

/** Smallest amount an online payment provider can charge. */
export const MIN_ONLINE_PAYMENT_AMOUNT = 0.01

export type CouponIneligibleReason =
  | 'locked'
  | 'unavailable'
  | 'threshold'
  | 'limit'

type SelectableCoupon = Pick<
  UserCoupon,
  'id' | 'threshold' | 'discount' | 'stackable' | 'status'
>

/** Round a settlement amount to 2 decimal places. */
export function roundCouponAmount(value: number): number {
  return Math.round((value + Number.EPSILON) * 100) / 100
}

/**
 * Explain why a coupon cannot be added to the current selection, or return
 * null when it can. Coupons already selected are always selectable so they
 * can be deselected.
 */
export function getCouponIneligibleReason(
  coupon: SelectableCoupon,
  selected: SelectableCoupon[],
  orderAmount: number
): CouponIneligibleReason | null {
  if (selected.some((item) => item.id === coupon.id)) return null
  if (coupon.status === 'locked') return 'locked'
  if (coupon.status !== 'available') return 'unavailable'
  if (coupon.threshold > orderAmount) return 'threshold'
  if (selected.length >= MAX_COUPONS_PER_ORDER) return 'limit'
  return null
}

/**
 * Toggle a coupon in the selection. Picking a non-stackable coupon replaces
 * the whole selection; picking a stackable one drops any non-stackable coupon
 * already selected. Ineligible coupons leave the selection unchanged.
 */
export function toggleCouponSelection<T extends SelectableCoupon>(
  selected: T[],
  coupon: T,
  orderAmount: number
): T[] {
  if (selected.some((item) => item.id === coupon.id)) {
    return selected.filter((item) => item.id !== coupon.id)
  }
  const stackableSelection = coupon.stackable
    ? selected.filter((item) => item.stackable)
    : []
  if (
    getCouponIneligibleReason(coupon, stackableSelection, orderAmount) !== null
  ) {
    return selected
  }
  return [...stackableSelection, coupon]
}

/**
 * Drop selected coupons that no longer apply to the order, e.g. after the
 * amount changed and a threshold is no longer met.
 */
export function pruneCouponSelection<T extends SelectableCoupon>(
  selected: T[],
  orderAmount: number
): T[] {
  const next = selected.filter(
    (item) => item.status === 'available' && item.threshold <= orderAmount
  )
  if (next.length > 1 && next.some((item) => !item.stackable)) {
    return next.filter((item) => item.stackable)
  }
  return next.slice(0, MAX_COUPONS_PER_ORDER)
}

export interface CouponDiscountSummary {
  /** Sum of the selected discounts (may exceed the order amount). */
  totalDiscount: number
  /** Amount actually taken off the order. */
  appliedDiscount: number
  /** max(orderAmount - totalDiscount, 0), rounded to 2 decimals. */
  finalAmount: number
}

export function calculateCouponDiscount(
  orderAmount: number,
  selected: Pick<UserCoupon, 'discount'>[]
): CouponDiscountSummary {
  const total = roundCouponAmount(
    selected.reduce((sum, item) => sum + item.discount, 0)
  )
  const finalAmount = roundCouponAmount(Math.max(orderAmount - total, 0))
  return {
    totalDiscount: total,
    appliedDiscount: roundCouponAmount(Math.max(orderAmount - finalAmount, 0)),
    finalAmount,
  }
}

/** Online payments cannot charge less than one cent. */
export function isOnlinePayableAmount(amount: number): boolean {
  return roundCouponAmount(amount) >= MIN_ONLINE_PAYMENT_AMOUNT
}

/** Quota needed to pay a subscription with balance after coupons. */
export function getBalanceQuotaCost(
  finalAmount: number,
  quotaPerUnit: number
): number {
  return Math.max(0, Math.ceil(roundCouponAmount(finalAmount) * quotaPerUnit))
}
