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
import { getRouteApi } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import {
  DISABLED_ROW_DESKTOP,
  DISABLED_ROW_MOBILE,
  DataTablePage,
  useDataTable,
} from '@/components/data-table'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { createServerError } from '@/lib/server-error-message'

import { getCoupons } from '../api'
import { COUPON_STATUS, ERROR_MESSAGES } from '../constants'
import { couponsQueryKey } from '../query-keys'
import { useCouponsColumns } from './coupons-columns'
import { CouponsMobileList } from './coupons-mobile-list'

const route = getRouteApi('/_authenticated/coupons/')

export function CouponsTable() {
  const { t } = useTranslation()
  const columns = useCouponsColumns()
  const isMobile = useMediaQuery('(max-width: 640px)')

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search: route.useSearch(),
    navigate: route.useNavigate(),
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
  })

  const page = pagination.pageIndex + 1
  const keyword = globalFilter?.trim() ?? ''

  const { data, isLoading, isFetching } = useQuery({
    queryKey: [...couponsQueryKey, page, pagination.pageSize, keyword],
    queryFn: async () => {
      const result = await getCoupons({
        p: page,
        page_size: pagination.pageSize,
        keyword,
      })
      if (!result.success) {
        throw createServerError(result, t(ERROR_MESSAGES.LOAD_FAILED))
      }
      return {
        items: result.data?.items ?? [],
        total: result.data?.total ?? 0,
      }
    },
    placeholderData: (previousData) => previousData,
  })

  const { table } = useDataTable({
    data: data?.items ?? [],
    columns,
    getRowId: (row) => String(row.id),
    columnFilters,
    globalFilter,
    pagination,
    onPaginationChange,
    onGlobalFilterChange,
    onColumnFiltersChange,
    manualPagination: true,
    manualFiltering: true,
    totalCount: data?.total ?? 0,
    ensurePageInRange,
  })

  return (
    <DataTablePage
      table={table}
      columns={columns}
      isLoading={isLoading}
      isFetching={isFetching}
      emptyTitle={t('No coupons found')}
      emptyDescription={t('Issued coupons will appear here.')}
      skeletonKeyPrefix='coupons-skeleton'
      applyHeaderSize
      toolbarProps={{
        searchPlaceholder: t('Filter by name...'),
        searchDebounceMs: 500,
      }}
      mobile={<CouponsMobileList table={table} isLoading={isLoading} />}
      getRowClassName={(row, ctx) => {
        if (row.original.status === COUPON_STATUS.ACTIVE) return undefined
        return ctx.isMobile ? DISABLED_ROW_MOBILE : DISABLED_ROW_DESKTOP
      }}
    />
  )
}
