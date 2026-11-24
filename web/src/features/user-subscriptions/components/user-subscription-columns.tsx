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
import { Settings2 } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import type { AdminUserSubscriptionRow } from '../types'
import {
  SubscriptionExpiryCell,
  SubscriptionPlanCell,
  SubscriptionStatusCell,
  SubscriptionUsageCell,
  SubscriptionUserCell,
} from './user-subscription-cells'

type UseUserSubscriptionColumnsOptions = {
  onManage: (row: AdminUserSubscriptionRow) => void
}

export function useUserSubscriptionColumns({
  onManage,
}: UseUserSubscriptionColumnsOptions): ColumnDef<AdminUserSubscriptionRow>[] {
  const { t } = useTranslation()

  return useMemo(
    (): ColumnDef<AdminUserSubscriptionRow>[] => [
      {
        id: 'user',
        header: t('User'),
        meta: { mobileTitle: true },
        cell: ({ row }) => <SubscriptionUserCell row={row.original} />,
        size: 220,
      },
      {
        id: 'plan',
        header: t('Plan'),
        cell: ({ row }) => <SubscriptionPlanCell row={row.original} />,
        size: 200,
      },
      {
        id: 'status',
        header: t('Status'),
        meta: { mobileBadge: true },
        cell: ({ row }) => <SubscriptionStatusCell row={row.original} />,
        size: 100,
      },
      {
        id: 'usage',
        header: t('Quota'),
        cell: ({ row }) => <SubscriptionUsageCell row={row.original} />,
        size: 180,
      },
      {
        id: 'expires_at',
        header: t('Expires'),
        cell: ({ row }) => <SubscriptionExpiryCell row={row.original} />,
        size: 180,
      },
      {
        id: 'actions',
        header: () => t('Actions'),
        meta: { pinned: 'right' as const },
        cell: ({ row }) => (
          <Tooltip>
            <TooltipTrigger
              render={
                <Button
                  variant='ghost'
                  size='icon-sm'
                  aria-label={t('Manage')}
                  onClick={() => onManage(row.original)}
                />
              }
            >
              <Settings2 />
            </TooltipTrigger>
            <TooltipContent>{t('Manage')}</TooltipContent>
          </Tooltip>
        ),
      },
    ],
    [t, onManage]
  )
}
