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

// ============================================================================
// Shared Coupon Types
// ============================================================================

/** Purchases a coupon can be applied to. */
export type CouponScope = 'topup' | 'subscription' | 'all'

/** Recipient selection mode used when a coupon batch is issued. */
export type CouponTargetType = 'all' | 'custom'

/** Status of a single coupon held by a user. */
export type UserCouponStatus =
  | 'available'
  | 'locked'
  | 'used'
  | 'expired'
  | 'revoked'

/** A coupon batch as managed by administrators. */
export interface Coupon {
  id: number
  name: string
  scope: CouponScope
  /** Minimum settlement amount; 0 means no minimum. */
  threshold: number
  /** Amount subtracted from the settlement amount. */
  discount: number
  stackable: boolean
  /** Unix seconds; 0 means the coupon never expires. */
  expires_at: number
  target_type: CouponTargetType
  target_user_ids: number[] | null
  target_groups: string[] | null
  /** 1: active, 2: revoked */
  status: number
  issued_count: number
  used_count: number
  created_by: number
  created_at: number
}

/** A user's copy of a coupon batch, as listed for administrators. */
export interface CouponRecipient {
  id: number
  user_id: number
  username: string
  status: UserCouponStatus
  used_at: number
  used_trade_no: string
  created_at: number
}

/** A coupon held by the current user (wallet and checkout). */
export interface UserCoupon {
  id: number
  coupon_id: number
  name: string
  scope: CouponScope
  threshold: number
  discount: number
  stackable: boolean
  expires_at: number
  status: UserCouponStatus
  used_at: number
  used_trade_no: string
  created_at: number
}

/** Filter accepted by GET /api/user/coupons. */
export type UserCouponFilter = 'available' | 'used' | 'expired'

/** Purchase type accepted by GET /api/user/coupons. */
export type UserCouponPurchase = 'topup' | 'subscription'

// ============================================================================
// API Request/Response Types
// ============================================================================

export interface ApiResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export interface PageData<T> {
  page: number
  page_size: number
  total: number
  items: T[] | null
}

export interface GetCouponsParams {
  p?: number
  page_size?: number
  keyword?: string
}

export interface CreateCouponPayload {
  name: string
  scope: CouponScope
  threshold: number
  discount: number
  stackable: boolean
  expires_at: number
  target_type: CouponTargetType
  user_ids: number[]
  groups: string[]
}

export interface CreateCouponResult {
  coupon: Coupon
  issued_count: number
}

export interface RevokeCouponResult {
  revoked_count: number
}

// ============================================================================
// Dialog Types
// ============================================================================

export type CouponsDialogType = 'create' | 'revoke' | 'recipients'
