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
import type { Table as TanstackTable } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { DISABLED_ROW_MOBILE } from '@/components/data-table'
import { EmptyState } from '@/components/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import { COUPON_STATUS } from '../constants'
import { getCouponRuleLabel, getCouponScopeLabel } from '../lib'
import type { Coupon } from '../types'
import { CouponExpiry, CouponStatusBadge } from './coupon-cells'
import { DataTableRowActions } from './data-table-row-actions'

const MOBILE_SKELETON_KEYS = [
  'coupon-mobile-skeleton-1',
  'coupon-mobile-skeleton-2',
  'coupon-mobile-skeleton-3',
  'coupon-mobile-skeleton-4',
]

interface CouponsMobileListProps {
  table: TanstackTable<Coupon>
  isLoading: boolean
}

export function CouponsMobileList(props: CouponsMobileListProps) {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const rows = props.table.getRowModel().rows

  if (props.isLoading) {
    return (
      <div className='divide-border overflow-hidden rounded-lg border'>
        {MOBILE_SKELETON_KEYS.map((key) => (
          <div
            key={key}
            className='space-y-2 border-b px-3 py-2.5 last:border-b-0'
          >
            <div className='flex items-center justify-between'>
              <Skeleton className='h-4 w-32' />
              <Skeleton className='h-5 w-16' />
            </div>
            <Skeleton className='h-3 w-40' />
            <Skeleton className='h-3 w-28' />
          </div>
        ))}
      </div>
    )
  }

  if (!rows.length) {
    return (
      <EmptyState
        bordered
        title={t('No coupons found')}
        description={t('Issued coupons will appear here.')}
      />
    )
  }

  return (
    <div className='divide-border overflow-hidden rounded-lg border'>
      {rows.map((row) => {
        const coupon = row.original
        return (
          <div
            key={row.id}
            className={cn(
              'bg-card space-y-2 border-b px-3 py-2.5 last:border-b-0',
              coupon.status !== COUPON_STATUS.ACTIVE && DISABLED_ROW_MOBILE
            )}
          >
            <div className='flex items-start justify-between gap-3'>
              <div className='min-w-0'>
                <div className='truncate text-sm font-semibold'>
                  {coupon.name}
                </div>
                <div className='text-muted-foreground text-xs'>
                  {getCouponScopeLabel(coupon.scope, t)} ·{' '}
                  {getCouponRuleLabel(coupon, t)}
                </div>
              </div>
              <CouponStatusBadge coupon={coupon} t={t} />
            </div>

            <div className='flex items-center justify-between gap-2 text-xs'>
              <span className='text-muted-foreground'>{t('Expires')}</span>
              <CouponExpiry expiresAt={coupon.expires_at} t={t} />
            </div>

            <div className='flex items-center justify-between gap-2 text-xs'>
              <span className='text-muted-foreground'>
                {t('Used / Issued')}
              </span>
              <span className='font-medium tabular-nums'>
                {formatNumber(coupon.used_count, locale)} /{' '}
                {formatNumber(coupon.issued_count, locale)}
              </span>
            </div>

            <div className='flex justify-end'>
              <DataTableRowActions coupon={coupon} />
            </div>
          </div>
        )
      })}
    </div>
  )
}
