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
import type { StatusBadgeProps } from '@/components/status-badge'

import type { CouponScope, UserCouponStatus } from './types'

// ============================================================================
// Coupon Batch Status (admin)
// ============================================================================

export const COUPON_STATUS = {
  ACTIVE: 1,
  REVOKED: 2,
} as const

// labelKey values are i18n keys; use t(config.labelKey) in components
export const COUPON_STATUSES: Record<
  number,
  Pick<StatusBadgeProps, 'variant'> & { labelKey: string }
> = {
  [COUPON_STATUS.ACTIVE]: { labelKey: 'Valid', variant: 'success' },
  [COUPON_STATUS.REVOKED]: { labelKey: 'Revoked', variant: 'neutral' },
}

// ============================================================================
// User Coupon Status
// ============================================================================

export const USER_COUPON_STATUSES: Record<
  UserCouponStatus,
  Pick<StatusBadgeProps, 'variant'> & { labelKey: string }
> = {
  available: { labelKey: 'Available', variant: 'success' },
  locked: { labelKey: 'Reserved by pending order', variant: 'neutral' },
  used: { labelKey: 'Used', variant: 'neutral' },
  expired: { labelKey: 'Expired', variant: 'neutral' },
  revoked: { labelKey: 'Revoked', variant: 'danger' },
}

// ============================================================================
// Scope
// ============================================================================

export const COUPON_SCOPE_LABELS: Record<CouponScope, string> = {
  topup: 'Balance Top-up',
  subscription: 'Membership subscription',
  all: 'All purchases',
}

export const COUPON_SCOPE_OPTIONS: CouponScope[] = [
  'topup',
  'subscription',
  'all',
]

// ============================================================================
// Validation
// ============================================================================

export const COUPON_VALIDATION = {
  NAME_MIN_LENGTH: 1,
  NAME_MAX_LENGTH: 64,
} as const

// ============================================================================
// Messages (i18n keys; use t(MESSAGES.xxx) when displaying)
// ============================================================================

export const ERROR_MESSAGES = {
  LOAD_FAILED: 'Failed to load coupons',
  LOAD_RECIPIENTS_FAILED: 'Failed to load coupon recipients',
  ISSUE_FAILED: 'Failed to issue coupons',
  REVOKE_FAILED: 'Failed to revoke coupon',
} as const
