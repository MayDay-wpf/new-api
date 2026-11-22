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
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
import { formatQuota } from '@/lib/format'

import {
  formatQuotaLimitWindow,
  getQuotaLimitUsage,
  quotaLimitKey,
} from '../lib/format'
import type { ResetScopeValue } from '../lib/reset-scope'
import type { SubscriptionPlan, UserSubscription } from '../types'

interface ResetScopeSelectorProps {
  plan?: Partial<SubscriptionPlan> | null
  // When provided, each row also shows the current usage of that subscription.
  subscription?: Pick<
    UserSubscription,
    'amount_total' | 'amount_used' | 'quota_limit_usage'
  > | null
  value: ResetScopeValue
  onChange: (value: ResetScopeValue) => void
}

export function ResetScopeSelector(props: ResetScopeSelectorProps) {
  const { t } = useTranslation()
  const limits = props.plan?.quota_limits || []
  // eslint-disable-next-line react-hooks/purity
  const now = Date.now() / 1000
  const totalAmount = Number(
    props.subscription?.amount_total ?? props.plan?.total_amount ?? 0
  )
  const totalUsed = Number(props.subscription?.amount_used ?? 0)

  const formatTotal = () => {
    if (totalAmount <= 0) return t('Unlimited')
    const total = formatQuota(totalAmount)
    if (!props.subscription) return total
    return `${formatQuota(totalUsed)}/${total}`
  }

  const toggleLimit = (key: string, checked: boolean) => {
    props.onChange({
      ...props.value,
      limitKeys: checked
        ? [...props.value.limitKeys, key]
        : props.value.limitKeys.filter((item) => item !== key),
    })
  }

  return (
    <div className='space-y-2'>
      <p className='text-sm font-medium'>{t('Reset scope')}</p>
      <label className='flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-sm'>
        <span className='flex items-center gap-2'>
          <Checkbox
            checked={props.value.resetTotal}
            onCheckedChange={(checked) =>
              props.onChange({ ...props.value, resetTotal: !!checked })
            }
          />
          <span>{t('Total Quota')}</span>
        </span>
        <span className='text-muted-foreground text-xs'>{formatTotal()}</span>
      </label>
      {limits.map((limit) => {
        const key = quotaLimitKey(limit)
        const amount = Number(limit.amount || 0)
        const used = props.subscription
          ? getQuotaLimitUsage(limit, props.subscription.quota_limit_usage, now)
          : 0
        return (
          <label
            key={key}
            className='flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-sm'
          >
            <span className='flex items-center gap-2'>
              <Checkbox
                checked={props.value.limitKeys.includes(key)}
                onCheckedChange={(checked) => toggleLimit(key, !!checked)}
              />
              <span>{formatQuotaLimitWindow(limit, t)}</span>
            </span>
            <span className='text-muted-foreground text-xs'>
              {props.subscription && amount > 0
                ? `${formatQuota(used)}/${formatQuota(amount)}`
                : formatQuota(amount)}
            </span>
          </label>
        )
      })}
      <p className='text-muted-foreground text-xs'>
        {t(
          'Only the selected counters are cleared. Other counters stay unchanged.'
        )}
      </p>
    </div>
  )
}
