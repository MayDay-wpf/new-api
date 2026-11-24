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
import type { ColumnDef } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import { TableId } from '@/components/table-id'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber, formatTimestampToDate } from '@/lib/format'

import { getCouponRuleLabel, getCouponScopeLabel } from '../lib'
import type { Coupon } from '../types'
import {
  CouponExpiry,
  CouponStatusBadge,
  CouponTargetCell,
} from './coupon-cells'
import { DataTableRowActions } from './data-table-row-actions'

export function useCouponsColumns(): ColumnDef<Coupon>[] {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)

  return [
    {
      accessorKey: 'id',
      header: t('ID'),
      meta: { mobileHidden: true },
      cell: ({ row }) => (
        <TableId value={row.original.id} className='w-[60px]' />
      ),
      size: 80,
    },
    {
      accessorKey: 'name',
      header: t('Name'),
      meta: { mobileTitle: true },
      cell: ({ row }) => (
        <span className='font-medium'>{row.original.name}</span>
      ),
      size: 180,
    },
    {
      accessorKey: 'scope',
      header: t('Applies to'),
      cell: ({ row }) => (
        <span className='text-sm whitespace-nowrap'>
          {getCouponScopeLabel(row.original.scope, t)}
        </span>
      ),
      size: 120,
    },
    {
      id: 'rule',
      header: t('Rule'),
      cell: ({ row }) => (
        <span className='text-sm whitespace-nowrap tabular-nums'>
          {getCouponRuleLabel(row.original, t)}
        </span>
      ),
      size: 160,
    },
    {
      accessorKey: 'stackable',
      header: t('Stackable'),
      cell: ({ row }) => (
        <span className='text-sm'>
          {row.original.stackable ? t('Yes') : t('No')}
        </span>
      ),
      size: 90,
    },
    {
      accessorKey: 'expires_at',
      header: t('Expires'),
      cell: ({ row }) => (
        <CouponExpiry expiresAt={row.original.expires_at} t={t} />
      ),
      size: 200,
    },
    {
      accessorKey: 'target_type',
      header: t('Recipients'),
      meta: { mobileHidden: true },
      cell: ({ row }) => <CouponTargetCell coupon={row.original} t={t} />,
      size: 120,
    },
    {
      id: 'usage',
      header: t('Used / Issued'),
      cell: ({ row }) => (
        <span className='font-mono text-sm tabular-nums'>
          {formatNumber(row.original.used_count, locale)} /{' '}
          {formatNumber(row.original.issued_count, locale)}
        </span>
      ),
      size: 120,
    },
    {
      accessorKey: 'status',
      header: t('Status'),
      meta: { mobileBadge: true },
      cell: ({ row }) => <CouponStatusBadge coupon={row.original} t={t} />,
      size: 100,
    },
    {
      accessorKey: 'created_at',
      header: t('Created'),
      meta: { mobileHidden: true },
      cell: ({ row }) => (
        <div className='min-w-[160px] font-mono text-sm'>
          {formatTimestampToDate(row.original.created_at)}
        </div>
      ),
      size: 180,
    },
    {
      id: 'actions',
      header: () => t('Actions'),
      cell: ({ row }) => <DataTableRowActions coupon={row.original} />,
      meta: { pinned: 'right' as const },
    },
  ]
}
