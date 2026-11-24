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
import type { ApiResponse } from '@/features/subscriptions/types'
import { api } from '@/lib/api'

import type {
  AdminUserSubscriptionPage,
  AdminUserSubscriptionQuery,
} from './types'

export async function getAdminUserSubscriptions(
  params: AdminUserSubscriptionQuery = {}
): Promise<ApiResponse<AdminUserSubscriptionPage>> {
  const query = new URLSearchParams({
    p: String(params.page ?? 1),
    page_size: String(params.pageSize ?? 20),
  })
  const keyword = params.keyword?.trim()
  if (keyword) query.set('keyword', keyword)
  if (params.planId) query.set('plan_id', String(params.planId))
  if (params.status) query.set('status', params.status)
  const res = await api.get(
    `/api/subscription/admin/user-subscriptions?${query.toString()}`
  )
  return res.data
}
