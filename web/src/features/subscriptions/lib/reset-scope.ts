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
import type { SubscriptionPlan } from '../types'
import { quotaLimitKey } from './format'

export interface ResetScopeValue {
  resetTotal: boolean
  limitKeys: string[]
}

// fullResetScope selects every counter a plan currently configures, matching
// the legacy reset behaviour.
export function fullResetScope(
  plan?: Partial<SubscriptionPlan> | null
): ResetScopeValue {
  return {
    resetTotal: true,
    limitKeys: (plan?.quota_limits || []).map((limit) => quotaLimitKey(limit)),
  }
}

export function isResetScopeEmpty(scope: ResetScopeValue): boolean {
  return !scope.resetTotal && scope.limitKeys.length === 0
}
