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
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback } from 'react'
import { useTranslation } from 'react-i18next'

import { toIntlLocale } from '@/i18n/languages'
import { formatNumber } from '@/lib/format'

import { getUnreadCount } from '../api'

export const TICKET_UNREAD_QUERY_KEY = ['ticket-unread-count'] as const

/** Invalidate the badge after opening a ticket, replying, or closing it. */
export function useInvalidateTicketUnread() {
  const queryClient = useQueryClient()
  return useCallback(
    () =>
      queryClient.invalidateQueries({ queryKey: TICKET_UNREAD_QUERY_KEY }),
    [queryClient]
  )
}

/** Formatted unread ticket count for sidebar badges, polled every 30 s. */
export function useTicketUnreadBadge(): string | undefined {
  const { i18n } = useTranslation()
  const { data } = useQuery({
    queryKey: TICKET_UNREAD_QUERY_KEY,
    queryFn: async () => {
      const res = await getUnreadCount()
      return res.success && res.data ? res.data.count : 0
    },
    refetchInterval: 30_000,
    staleTime: 10_000,
  })
  const count = data ?? 0
  if (count === 0) return undefined
  return formatNumber(count, toIntlLocale(i18n.resolvedLanguage || i18n.language))
}
