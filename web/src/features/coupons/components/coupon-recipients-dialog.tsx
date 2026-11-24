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
import type { ColumnDef, PaginationState } from '@tanstack/react-table'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DataTablePagination,
  DataTableView,
  useDataTable,
} from '@/components/data-table'
import { Dialog } from '@/components/dialog'
import { StatusBadge } from '@/components/status-badge'
import { formatTimestampToDate } from '@/lib/format'
import { createServerError } from '@/lib/server-error-message'

import { getCouponRecipients } from '../api'
import { ERROR_MESSAGES, USER_COUPON_STATUSES } from '../constants'
import { couponsQueryKey } from '../query-keys'
import type { CouponRecipient } from '../types'
import { useCoupons } from './coupons-provider'

const DEFAULT_PAGINATION: PaginationState = { pageIndex: 0, pageSize: 10 }

function useRecipientColumns(): ColumnDef<CouponRecipient>[] {
  const { t } = useTranslation()
  return useMemo(
    () => [
      {
        accessorKey: 'user_id',
        header: t('User ID'),
        cell: ({ row }) => (
          <span className='font-mono text-sm'>{row.original.user_id}</span>
        ),
      },
      {
        accessorKey: 'username',
        header: t('Username'),
        cell: ({ row }) => (
          <span className='text-sm'>{row.original.username || '-'}</span>
        ),
      },
      {
        accessorKey: 'status',
        header: t('Status'),
        cell: ({ row }) => {
          const config = USER_COUPON_STATUSES[row.original.status]
          if (!config) return null
          return (
            <StatusBadge
              label={t(config.labelKey)}
              variant={config.variant}
              copyable={false}
            />
          )
        },
      },
      {
        accessorKey: 'used_at',
        header: t('Used at'),
        cell: ({ row }) => (
          <span className='font-mono text-sm whitespace-nowrap'>
            {formatTimestampToDate(row.original.used_at)}
          </span>
        ),
      },
      {
        accessorKey: 'used_trade_no',
        header: t('Order number'),
        cell: ({ row }) => (
          <span className='font-mono text-xs break-all'>
            {row.original.used_trade_no || '-'}
          </span>
        ),
      },
      {
        accessorKey: 'created_at',
        header: t('Received at'),
        cell: ({ row }) => (
          <span className='font-mono text-sm whitespace-nowrap'>
            {formatTimestampToDate(row.original.created_at)}
          </span>
        ),
      },
    ],
    [t]
  )
}

export function CouponRecipientsDialog() {
  const { t } = useTranslation()
  const { open, setOpen, currentRow } = useCoupons()
  const isOpen = open === 'recipients' && currentRow != null
  const couponId = currentRow?.id ?? 0
  const columns = useRecipientColumns()
  const [pagination, setPagination] =
    useState<PaginationState>(DEFAULT_PAGINATION)

  useEffect(() => {
    setPagination(DEFAULT_PAGINATION)
  }, [couponId])

  const { data, isLoading } = useQuery({
    queryKey: [
      ...couponsQueryKey,
      'recipients',
      couponId,
      pagination.pageIndex + 1,
      pagination.pageSize,
    ],
    queryFn: async () => {
      const result = await getCouponRecipients(couponId, {
        p: pagination.pageIndex + 1,
        page_size: pagination.pageSize,
      })
      if (!result.success) {
        throw createServerError(
          result,
          t(ERROR_MESSAGES.LOAD_RECIPIENTS_FAILED)
        )
      }
      return {
        items: result.data?.items ?? [],
        total: result.data?.total ?? 0,
      }
    },
    enabled: isOpen,
    placeholderData: (previousData) => previousData,
  })

  const { table } = useDataTable({
    data: data?.items ?? [],
    columns,
    getRowId: (row) => String(row.id),
    pagination,
    onPaginationChange: setPagination,
    manualPagination: true,
    totalCount: data?.total ?? 0,
    columnVisibilityStorageKey: false,
    columnSizingStorageKey: false,
  })

  return (
    <Dialog
      open={isOpen}
      onOpenChange={(next) => !next && setOpen(null)}
      title={t('Coupon recipients')}
      description={currentRow?.name}
      contentClassName='flex max-h-(--dialog-available-height) flex-col sm:max-w-4xl'
      contentHeight='auto'
      bodyClassName='space-y-3'
    >
      <DataTableView
        table={table}
        isLoading={isLoading}
        skeletonKeyPrefix='coupon-recipients-skeleton'
        tableHeaderClassName='[background-color:var(--table-header)]'
        emptyTitle={t('No recipients')}
        emptyDescription={t('No users have received this coupon.')}
      />
      <DataTablePagination table={table} compact />
    </Dialog>
  )
}
