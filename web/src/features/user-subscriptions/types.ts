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
import type {
  PlanQuotaLimit,
  UserSubscription,
} from '@/features/subscriptions/types'

/** One user subscription joined with the user and plan display fields. */
export interface AdminUserSubscriptionRow {
  subscription: UserSubscription
  username: string
  display_name: string
  email: string
  plan_title: string
  plan_subtitle: string
  /** Periodic quota windows of the plan (e.g. a 5h window). */
  plan_quota_limits?: PlanQuotaLimit[] | null
}

/** Overview counts shown above the list (status filter not applied). */
export interface AdminUserSubscriptionSummary {
  total: number
  active: number
  expiring_soon: number
}

export type AdminUserSubscriptionStatus = 'active' | 'expired' | 'cancelled'

export interface AdminUserSubscriptionPage {
  page: number
  page_size: number
  total: number
  items: AdminUserSubscriptionRow[]
  summary: AdminUserSubscriptionSummary
}

export interface AdminUserSubscriptionQuery {
  page?: number
  pageSize?: number
  keyword?: string
  planId?: number
  status?: AdminUserSubscriptionStatus
}
