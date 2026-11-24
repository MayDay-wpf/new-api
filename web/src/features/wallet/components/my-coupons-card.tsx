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
import { ChevronLeft, ChevronRight, TicketPercent } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { TitledCard } from '@/components/ui/titled-card'
import { ToggleGroup, ToggleGroupItem } from '@/components/ui/toggle-group'
import { getUserCoupons } from '@/features/coupons/api'
import { USER_COUPON_STATUSES } from '@/features/coupons/constants'
import { getCouponRuleLabel, getCouponScopeLabel } from '@/features/coupons/lib'
import { userCouponsQueryKey } from '@/features/coupons/query-keys'
import type { UserCoupon, UserCouponFilter } from '@/features/coupons/types'
import { formatTimestampToDate } from '@/lib/format'
import { requireServerSuccess } from '@/lib/server-error-message'

const PAGE_SIZE = 10

const FILTERS: { value: UserCouponFilter; labelKey: string }[] = [
  { value: 'available', labelKey: 'Available' },
  { value: 'used', labelKey: 'Used' },
  { value: 'expired', labelKey: 'Expired' },
]

const EMPTY_TITLES: Record<UserCouponFilter, string> = {
  available: 'No available coupons',
  used: 'No used coupons',
  expired: 'No expired coupons',
}

function CouponMeta(props: { coupon: UserCoupon; t: TFunction }) {
  const { coupon, t } = props
  if (coupon.status === 'used') {
    return (
      <>
        <span>
          {t('Used at')}: {formatTimestampToDate(coupon.used_at)}
        </span>
        {coupon.used_trade_no && (
          <span className='font-mono break-all'>
            {t('Order number')}: {coupon.used_trade_no}
          </span>
        )}
      </>
    )
  }
  return (
    <span>
      {coupon.expires_at > 0
        ? t('Expires {{time}}', {
            time: formatTimestampToDate(coupon.expires_at),
          })
        : t('Permanent')}
    </span>
  )
}

function CouponRow(props: { coupon: UserCoupon }) {
  const { t } = useTranslation()
  const coupon = props.coupon
  const status = USER_COUPON_STATUSES[coupon.status]
  return (
    <li className='flex flex-col gap-2 px-3 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-4'>
      <div className='min-w-0 space-y-1'>
        <div className='flex flex-wrap items-center gap-2'>
          <span className='truncate text-sm font-medium'>{coupon.name}</span>
          {status && (
            <StatusBadge
              label={t(status.labelKey)}
              variant={status.variant}
              copyable={false}
            />
          )}
        </div>
        <div className='text-muted-foreground flex flex-wrap gap-x-3 gap-y-1 text-xs'>
          <span>{getCouponScopeLabel(coupon.scope, t)}</span>
          <span>{coupon.stackable ? t('Stackable') : t('Not stackable')}</span>
          <CouponMeta coupon={coupon} t={t} />
        </div>
      </div>
      <span className='shrink-0 text-sm font-semibold tabular-nums'>
        {getCouponRuleLabel(coupon, t)}
      </span>
    </li>
  )
}

export function MyCouponsCard() {
  const { t } = useTranslation()
  const [filter, setFilter] = useState<UserCouponFilter>('available')
  const [page, setPage] = useState(1)

  const { data, isLoading } = useQuery({
    queryKey: [...userCouponsQueryKey, 'list', filter, page],
    queryFn: async () => {
      const result = requireServerSuccess(
        await getUserCoupons({ status: filter, p: page, page_size: PAGE_SIZE })
      )
      return {
        items: result.data?.items ?? [],
        total: result.data?.total ?? 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const items = data?.items ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  return (
    <TitledCard
      title={t('My Coupons')}
      icon={<TicketPercent className='h-4 w-4' />}
      disableHoverEffect
      action={
        <ToggleGroup
          value={[filter]}
          onValueChange={(values) => {
            const next = values.find((value) => value !== filter)
            if (next) {
              setFilter(next as UserCouponFilter)
              setPage(1)
            }
          }}
          aria-label={t('Coupon status')}
          variant='outline'
          size='sm'
        >
          {FILTERS.map((option) => (
            <ToggleGroupItem key={option.value} value={option.value}>
              {t(option.labelKey)}
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
      }
      contentClassName='p-0'
    >
      {isLoading && (
        <div className='space-y-3 p-4'>
          <Skeleton className='h-12 w-full' />
          <Skeleton className='h-12 w-full' />
          <Skeleton className='h-12 w-full' />
        </div>
      )}

      {!isLoading && items.length === 0 && (
        <EmptyState
          icon={TicketPercent}
          title={t(EMPTY_TITLES[filter])}
          className='min-h-[220px]'
        />
      )}

      {!isLoading && items.length > 0 && (
        <>
          <ul className='divide-y'>
            {items.map((coupon) => (
              <CouponRow key={coupon.id} coupon={coupon} />
            ))}
          </ul>
          {totalPages > 1 && (
            <nav
              aria-label={t('Page')}
              className='flex items-center justify-between gap-2 border-t px-3 py-2 text-sm sm:px-4'
            >
              <span className='text-muted-foreground'>
                {t('Total:')} {total}
              </span>
              <div className='flex items-center gap-2'>
                <Button
                  variant='outline'
                  size='icon-sm'
                  aria-label={t('Go to previous page')}
                  disabled={page <= 1}
                  onClick={() => setPage((value) => value - 1)}
                >
                  <ChevronLeft />
                </Button>
                <span className='text-muted-foreground tabular-nums'>
                  {page} / {totalPages}
                </span>
                <Button
                  variant='outline'
                  size='icon-sm'
                  aria-label={t('Go to next page')}
                  disabled={page >= totalPages}
                  onClick={() => setPage((value) => value + 1)}
                >
                  <ChevronRight />
                </Button>
              </div>
            </nav>
          )}
        </>
      )}
    </TitledCard>
  )
}
