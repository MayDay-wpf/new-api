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
import { api } from '@/lib/api'

import type {
  ApiResponse,
  Coupon,
  CouponRecipient,
  CreateCouponPayload,
  CreateCouponResult,
  GetCouponsParams,
  PageData,
  RevokeCouponResult,
  UserCoupon,
  UserCouponFilter,
  UserCouponPurchase,
} from './types'

// ============================================================================
// Coupon Management (admin)
// ============================================================================

export async function getCoupons(
  params: GetCouponsParams = {}
): Promise<ApiResponse<PageData<Coupon>>> {
  const query = new URLSearchParams({
    p: String(params.p ?? 1),
    page_size: String(params.page_size ?? 20),
  })
  const keyword = params.keyword?.trim()
  if (keyword) query.set('keyword', keyword)
  const res = await api.get(`/api/coupon/?${query.toString()}`)
  return res.data
}

export async function createCoupon(
  payload: CreateCouponPayload
): Promise<ApiResponse<CreateCouponResult>> {
  const res = await api.post('/api/coupon/', payload)
  return res.data
}

export async function revokeCoupon(
  id: number
): Promise<ApiResponse<RevokeCouponResult>> {
  const res = await api.post(`/api/coupon/${id}/revoke`)
  return res.data
}

// ============================================================================
// Current User Coupons
// ============================================================================

export async function getUserCoupons(params: {
  status: UserCouponFilter
  scope?: UserCouponPurchase
  p?: number
  page_size?: number
}): Promise<ApiResponse<PageData<UserCoupon>>> {
  const query = new URLSearchParams({
    status: params.status,
    p: String(params.p ?? 1),
    page_size: String(params.page_size ?? 20),
  })
  if (params.scope) query.set('scope', params.scope)
  const res = await api.get(`/api/user/coupons?${query.toString()}`)
  return res.data
}

export async function getCouponRecipients(
  id: number,
  params: { p?: number; page_size?: number } = {}
): Promise<ApiResponse<PageData<CouponRecipient>>> {
  const query = new URLSearchParams({
    p: String(params.p ?? 1),
    page_size: String(params.page_size ?? 20),
  })
  const res = await api.get(`/api/coupon/${id}/users?${query.toString()}`)
  return res.data
}
