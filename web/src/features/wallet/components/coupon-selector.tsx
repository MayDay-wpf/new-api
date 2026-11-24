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
import { useQuery } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
import { Skeleton } from '@/components/ui/skeleton'
import { getUserCoupons } from '@/features/coupons/api'
import { getCouponRuleLabel } from '@/features/coupons/lib'
import { userCouponsQueryKey } from '@/features/coupons/query-keys'
import type { UserCoupon, UserCouponPurchase } from '@/features/coupons/types'
import { formatTimestampToDate } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'
import { cn } from '@/lib/utils'

import {
  type CouponIneligibleReason,
  getCouponIneligibleReason,
  pruneCouponSelection,
  toggleCouponSelection,
} from '../lib/coupon'

/** Checkout lists every usable coupon at once; users rarely hold more. */
const CHECKOUT_COUPON_PAGE_SIZE = 100

function getReasonLabel(reason: CouponIneligibleReason, t: TFunction): string {
  switch (reason) {
    case 'locked':
      return t('Reserved by pending order')
    case 'threshold':
      return t('Order amount does not meet the coupon threshold')
    case 'limit':
      return t('Too many coupons selected')
    default:
      return t('Coupon not found or unavailable')
  }
}

/** Load the current user's usable coupons for one purchase type. */
function useCheckoutCoupons(purchase: UserCouponPurchase, enabled: boolean) {
  return useQuery({
    queryKey: [...userCouponsQueryKey, 'checkout', purchase],
    queryFn: async () => {
      const result = requireServerSuccess(
        await getUserCoupons({
          status: 'available',
          scope: purchase,
          page_size: CHECKOUT_COUPON_PAGE_SIZE,
        })
      )
      return result.data?.items ?? []
    },
    enabled,
    staleTime: 0,
  })
}

interface CouponSelectorProps {
  purchase: UserCouponPurchase
  /** Settlement amount before coupons. */
  orderAmount: number
  selected: UserCoupon[]
  onChange: (selected: UserCoupon[]) => void
  /** Load and show coupons only while the checkout is open. */
  enabled: boolean
  disabled?: boolean
  className?: string
}

export function CouponSelector(props: CouponSelectorProps) {
  const { t } = useTranslation()
  const { data: coupons = [], isLoading } = useCheckoutCoupons(
    props.purchase,
    props.enabled
  )
  const { selected, orderAmount, onChange } = props

  // Keep the selection valid when the order amount changes.
  useEffect(() => {
    const next = pruneCouponSelection(selected, orderAmount)
    if (next.length !== selected.length) onChange(next)
  }, [selected, orderAmount, onChange])

  if (!props.enabled) return null

  return (
    <div className={cn('space-y-2', props.className)}>
      <div className='flex items-center justify-between gap-2'>
        <span className='text-muted-foreground text-sm'>{t('Coupons')}</span>
        {selected.length > 0 && (
          <span className='text-muted-foreground text-xs'>
            {t('{{count}} coupons selected', { count: selected.length })}
          </span>
        )}
      </div>

      {isLoading && <Skeleton className='h-14 w-full' />}

      {!isLoading && coupons.length === 0 && (
        <p className='text-muted-foreground rounded-md border px-3 py-2 text-xs'>
          {t('No coupons available')}
        </p>
      )}

      {!isLoading && coupons.length > 0 && (
        <ul
          className='max-h-48 divide-y overflow-y-auto rounded-md border'
          aria-label={t('Coupons')}
        >
          {coupons.map((coupon) => {
            const checked = selected.some((item) => item.id === coupon.id)
            const reason = getCouponIneligibleReason(
              coupon,
              coupon.stackable ? selected.filter((item) => item.stackable) : [],
              orderAmount
            )
            const itemDisabled = props.disabled || reason !== null
            const inputId = `checkout-coupon-${coupon.id}`
            return (
              <li
                key={coupon.id}
                className={cn(
                  'flex items-start gap-3 px-3 py-2',
                  reason !== null && 'opacity-60'
                )}
              >
                <Checkbox
                  id={inputId}
                  checked={checked}
                  disabled={itemDisabled}
                  onCheckedChange={() =>
                    onChange(
                      toggleCouponSelection(selected, coupon, orderAmount)
                    )
                  }
                  className='mt-0.5'
                />
                <label
                  htmlFor={inputId}
                  className='min-w-0 flex-1 cursor-pointer text-sm'
                >
                  <span className='flex items-center justify-between gap-2'>
                    <span className='truncate font-medium'>{coupon.name}</span>
                    <span className='shrink-0 tabular-nums'>
                      {getCouponRuleLabel(coupon, t)}
                    </span>
                  </span>
                  <span className='text-muted-foreground mt-0.5 flex flex-wrap gap-x-2 text-xs'>
                    <span>
                      {coupon.stackable ? t('Stackable') : t('Not stackable')}
                    </span>
                    <span>
                      {coupon.expires_at > 0
                        ? t('Expires {{time}}', {
                            time: formatTimestampToDate(coupon.expires_at),
                          })
                        : t('Permanent')}
                    </span>
                    {reason !== null && (
                      <span>{getReasonLabel(reason, t)}</span>
                    )}
                  </span>
                </label>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}
