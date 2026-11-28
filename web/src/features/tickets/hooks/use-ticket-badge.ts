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

export type TicketUnreadBadges = {
  /** Own tickets with unread admin replies (personal "Support Tickets") */
  user?: string
  /** Tickets with unread user messages (admin "Tickets") */
  admin?: string
}

/** Formatted unread ticket counts for sidebar badges, polled every 30 s. */
export function useTicketUnreadBadge(): TicketUnreadBadges {
  const { i18n } = useTranslation()
  const { data } = useQuery({
    queryKey: TICKET_UNREAD_QUERY_KEY,
    queryFn: async () => {
      const res = await getUnreadCount()
      return res.success && res.data
        ? { user: res.data.user_count ?? 0, admin: res.data.admin_count ?? 0 }
        : { user: 0, admin: 0 }
    },
    refetchInterval: 30_000,
    staleTime: 10_000,
  })
  const locale = toIntlLocale(i18n.resolvedLanguage || i18n.language)
  const format = (count: number | undefined) =>
    count ? formatNumber(count, locale) : undefined
  return { user: format(data?.user), admin: format(data?.admin) }
}
