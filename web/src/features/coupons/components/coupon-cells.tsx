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
import type { TFunction } from 'i18next'

import { StatusBadge } from '@/components/status-badge'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatTimestampToDate } from '@/lib/format'
import { cn } from '@/lib/utils'

import { COUPON_STATUSES } from '../constants'
import { isCouponExpired } from '../lib'
import type { Coupon } from '../types'

export function CouponStatusBadge(props: { coupon: Coupon; t: TFunction }) {
  const config = COUPON_STATUSES[props.coupon.status]
  if (!config) return null
  return (
    <StatusBadge
      label={props.t(config.labelKey)}
      variant={config.variant}
      copyable={false}
      className='-ml-1.5'
    />
  )
}

export function CouponExpiry(props: { expiresAt: number; t: TFunction }) {
  if (props.expiresAt === 0) {
    return (
      <span className='text-muted-foreground text-sm'>
        {props.t('Permanent')}
      </span>
    )
  }
  const expired = isCouponExpired(props.expiresAt)
  return (
    <span
      className={cn(
        'font-mono text-sm whitespace-nowrap',
        expired && 'text-destructive'
      )}
    >
      {formatTimestampToDate(props.expiresAt)}
      {expired && ` · ${props.t('Expired')}`}
    </span>
  )
}

export function CouponTargetCell(props: { coupon: Coupon; t: TFunction }) {
  if (props.coupon.target_type === 'all') {
    return <span className='text-sm'>{props.t('All users')}</span>
  }
  const userIds = props.coupon.target_user_ids ?? []
  const groups = props.coupon.target_groups ?? []
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <span className='cursor-help text-sm'>
            {props.t('Specified recipients')}
          </span>
        }
      />
      <TooltipContent>
        <div className='max-w-xs space-y-1 text-xs'>
          {userIds.length > 0 && (
            <div>
              {props.t('User IDs')}: {userIds.join(', ')}
            </div>
          )}
          {groups.length > 0 && (
            <div>
              {props.t('Groups')}: {groups.join(', ')}
            </div>
          )}
        </div>
      </TooltipContent>
    </Tooltip>
  )
}
