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
import { useCallback, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
import { Card, CardContent } from '@/components/ui/card'
import { NativeSelect, NativeSelectOption } from '@/components/ui/native-select'
import { getAdminPlans } from '@/features/subscriptions/api'
import { UserSubscriptionsDialog } from '@/features/subscriptions/components/dialogs/user-subscriptions-dialog'
import { useMediaQuery } from '@/hooks'
import { useTableUrlState } from '@/hooks/use-table-url-state'
import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'
import { createServerError } from '@/lib/server-error-message'

import { getAdminUserSubscriptions } from '../api'
import type {
  AdminUserSubscriptionRow,
  AdminUserSubscriptionStatus,
} from '../types'
import { useUserSubscriptionColumns } from './user-subscription-columns'

const route = getRouteApi('/_authenticated/users/subscriptions')

const STATUS_OPTIONS: AdminUserSubscriptionStatus[] = [
  'active',
  'expired',
  'cancelled',
]

const STATUS_LABELS: Record<AdminUserSubscriptionStatus, string> = {
  active: 'Active',
  expired: 'Expired',
  cancelled: 'Cancelled',
}

type ManageTarget = { id: number; username?: string } | null

function SubscriptionSummaryCard(props: {
  label: string
  value: number
  loading?: boolean
  locale: Intl.LocalesArgument
}) {
  return (
    <Card size='sm'>
      <CardContent>
        <div className='text-muted-foreground text-xs'>{props.label}</div>
        <div className='mt-1 text-xl font-semibold tabular-nums'>
          {props.loading ? '-' : formatNumber(props.value, props.locale)}
        </div>
      </CardContent>
    </Card>
  )
}

export function UserSubscriptionsTable() {
  const { t, i18n } = useTranslation()
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const isMobile = useMediaQuery('(max-width: 640px)')
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const [manageUser, setManageUser] = useState<ManageTarget>(null)

  const {
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    ensurePageInRange,
  } = useTableUrlState({
    search,
    navigate,
    pagination: { defaultPage: 1, defaultPageSize: isMobile ? 10 : 20 },
    globalFilter: { enabled: true, key: 'filter' },
  })

  const page = pagination.pageIndex + 1
  const keyword = globalFilter?.trim() ?? ''
  const planId = search.planId
  const status = search.status

  const { data, isLoading, isFetching, refetch } = useQuery({
    queryKey: [
      'admin-user-subscriptions',
      page,
      pagination.pageSize,
      keyword,
      planId ?? 0,
      status ?? '',
    ],
    queryFn: async () => {
      const result = await getAdminUserSubscriptions({
        page,
        pageSize: pagination.pageSize,
        keyword,
        planId,
        status,
      })
      if (!result.success) {
        throw createServerError(result, t('Failed to load'))
      }
      return result.data
    },
    placeholderData: (previousData) => previousData,
  })

  const plansQuery = useQuery({
    queryKey: ['admin-subscription-plans'],
    queryFn: async () => {
      const result = await getAdminPlans()
      if (!result.success) {
        throw createServerError(result, t('Failed to load'))
      }
      return result.data ?? []
    },
    staleTime: 60_000,
  })

  const handleManage = useCallback((row: AdminUserSubscriptionRow) => {
    setManageUser({
      id: row.subscription.user_id,
      username: row.username || row.display_name,
    })
  }, [])

  const columns = useUserSubscriptionColumns({ onManage: handleManage })

  const { table } = useDataTable({
    data: data?.items ?? [],
    columns,
    getRowId: (row) => String(row.subscription.id),
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

  const setPlanId = (value?: number) => {
    navigate({
      search: (prev) => ({ ...prev, page: undefined, planId: value }),
    })
  }

  const setStatus = (value?: AdminUserSubscriptionStatus) => {
    navigate({
      search: (prev) => ({ ...prev, page: undefined, status: value }),
    })
  }

  const summary = data?.summary

  return (
    <div className='flex h-full min-h-0 flex-col gap-4'>
      <div className='grid shrink-0 grid-cols-1 gap-3 sm:grid-cols-3'>
        <SubscriptionSummaryCard
          label={t('Total subscriptions')}
          value={summary?.total ?? 0}
          loading={isLoading}
          locale={locale}
        />
        <SubscriptionSummaryCard
          label={t('Active subscriptions')}
          value={summary?.active ?? 0}
          loading={isLoading}
          locale={locale}
        />
        <SubscriptionSummaryCard
          label={t('Expiring within 7 days')}
          value={summary?.expiring_soon ?? 0}
          loading={isLoading}
          locale={locale}
        />
      </div>

      <div className='min-h-0 flex-1'>
        <DataTablePage
          table={table}
          columns={columns}
          isLoading={isLoading}
          isFetching={isFetching}
          emptyTitle={t('No user subscriptions found')}
          emptyDescription={t('User subscriptions will appear here.')}
          skeletonKeyPrefix='user-subscriptions-skeleton'
          toolbarProps={{
            searchPlaceholder: t('Search by user, email or plan...'),
            searchDebounceMs: 500,
            collapsibleOnMobile: true,
            additionalSearch: (
              <>
                <NativeSelect
                  size='sm'
                  className='w-[160px]'
                  aria-label={t('Plan')}
                  value={planId ? String(planId) : ''}
                  onChange={(event) =>
                    setPlanId(
                      event.target.value
                        ? Number(event.target.value)
                        : undefined
                    )
                  }
                >
                  <NativeSelectOption value=''>
                    {t('All Plans')}
                  </NativeSelectOption>
                  {(plansQuery.data ?? []).map((record) => (
                    <NativeSelectOption
                      key={record.plan.id}
                      value={String(record.plan.id)}
                    >
                      {record.plan.title}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
                <NativeSelect
                  size='sm'
                  className='w-[150px]'
                  aria-label={t('Status')}
                  value={status ?? ''}
                  onChange={(event) =>
                    setStatus(
                      (event.target.value || undefined) as
                        | AdminUserSubscriptionStatus
                        | undefined
                    )
                  }
                >
                  <NativeSelectOption value=''>
                    {t('All statuses')}
                  </NativeSelectOption>
                  {STATUS_OPTIONS.map((option) => (
                    <NativeSelectOption key={option} value={option}>
                      {t(STATUS_LABELS[option])}
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </>
            ),
            hasAdditionalFilters: Boolean(planId || status),
            onReset: () => {
              navigate({
                search: (prev) => ({
                  ...prev,
                  page: undefined,
                  planId: undefined,
                  status: undefined,
                }),
              })
            },
          }}
        />
      </div>

      <UserSubscriptionsDialog
        open={manageUser !== null}
        onOpenChange={(open) => {
          if (!open) setManageUser(null)
        }}
        user={manageUser}
        onSuccess={() => {
          void refetch()
        }}
      />
    </div>
  )
}
