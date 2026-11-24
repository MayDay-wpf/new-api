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
import { describe, expect, test } from 'vitest'

import type { UserCoupon } from '@/features/coupons/types'

import {
  MAX_COUPONS_PER_ORDER,
  calculateCouponDiscount,
  getBalanceQuotaCost,
  getCouponIneligibleReason,
  isOnlinePayableAmount,
  pruneCouponSelection,
  toggleCouponSelection,
} from '../coupon'

function coupon(id: number, overrides: Partial<UserCoupon> = {}): UserCoupon {
  return {
    id,
    coupon_id: id,
    name: `Coupon ${id}`,
    scope: 'all',
    threshold: 0,
    discount: 1,
    stackable: true,
    expires_at: 0,
    status: 'available',
    used_at: 0,
    used_trade_no: '',
    created_at: 0,
    ...overrides,
  }
}

describe('coupon eligibility', () => {
  test('a coupon whose threshold exceeds the order amount is not selectable', () => {
    const reason = getCouponIneligibleReason(
      coupon(1, { threshold: 50 }),
      [],
      49.99
    )
    expect(reason).toBe('threshold')
  })

  test('a coupon whose threshold equals the order amount is selectable', () => {
    expect(
      getCouponIneligibleReason(coupon(1, { threshold: 50 }), [], 50)
    ).toBeNull()
  })

  test('a coupon reserved by a pending order is not selectable', () => {
    expect(
      getCouponIneligibleReason(coupon(1, { status: 'locked' }), [], 100)
    ).toBe('locked')
  })

  test('the selection cannot grow beyond the per-order coupon limit', () => {
    const selected = Array.from({ length: MAX_COUPONS_PER_ORDER }, (_, i) =>
      coupon(i + 1)
    )
    expect(getCouponIneligibleReason(coupon(99), selected, 100)).toBe('limit')
  })
})

describe('coupon selection toggling', () => {
  test('selecting a non-stackable coupon clears the other selected coupons', () => {
    const selected = [coupon(1), coupon(2)]
    const next = toggleCouponSelection(
      selected,
      coupon(3, { stackable: false }),
      100
    )
    expect(next.map((item) => item.id)).toEqual([3])
  })

  test('selecting a stackable coupon drops a selected non-stackable coupon', () => {
    const selected = [coupon(1, { stackable: false })]
    const next = toggleCouponSelection(selected, coupon(2), 100)
    expect(next.map((item) => item.id)).toEqual([2])
  })

  test('stackable coupons accumulate in the selection', () => {
    const next = toggleCouponSelection([coupon(1)], coupon(2), 100)
    expect(next.map((item) => item.id)).toEqual([1, 2])
  })

  test('toggling a selected coupon removes it', () => {
    const next = toggleCouponSelection([coupon(1), coupon(2)], coupon(1), 100)
    expect(next.map((item) => item.id)).toEqual([2])
  })

  test('toggling an ineligible coupon leaves the selection unchanged', () => {
    const selected = [coupon(1)]
    const next = toggleCouponSelection(
      selected,
      coupon(2, { threshold: 200 }),
      100
    )
    expect(next).toBe(selected)
  })

  test('a lowered order amount prunes coupons whose threshold is no longer met', () => {
    const selected = [coupon(1), coupon(2, { threshold: 80 })]
    expect(pruneCouponSelection(selected, 50).map((item) => item.id)).toEqual([
      1,
    ])
  })
})

describe('coupon discount calculation', () => {
  test('the final amount subtracts every selected discount and rounds to cents', () => {
    const summary = calculateCouponDiscount(10.1, [
      coupon(1, { discount: 0.2 }),
      coupon(2, { discount: 0.1 }),
    ])
    expect(summary).toEqual({
      totalDiscount: 0.3,
      appliedDiscount: 0.3,
      finalAmount: 9.8,
    })
  })

  test('discounts larger than the order amount floor the final amount at zero', () => {
    const summary = calculateCouponDiscount(5, [coupon(1, { discount: 8 })])
    expect(summary.finalAmount).toBe(0)
    expect(summary.appliedDiscount).toBe(5)
  })

  test('an empty selection keeps the order amount', () => {
    expect(calculateCouponDiscount(12.34, []).finalAmount).toBe(12.34)
  })

  test('online payments require at least one cent while zero is rejected', () => {
    expect(isOnlinePayableAmount(0.01)).toBe(true)
    expect(isOnlinePayableAmount(0)).toBe(false)
  })

  test('balance payments round the required quota up', () => {
    expect(getBalanceQuotaCost(1.005, 500000)).toBe(505000)
    expect(getBalanceQuotaCost(0, 500000)).toBe(0)
  })
})
