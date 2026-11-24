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
import { useQueryClient } from '@tanstack/react-query'
import { Crown, CalendarClock, Package } from 'lucide-react'
import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { Dialog } from '@/components/dialog'
import { GroupBadge } from '@/components/group-badge'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import { Separator } from '@/components/ui/separator'
import { userCouponsQueryKey } from '@/features/coupons/query-keys'
import type { UserCoupon } from '@/features/coupons/types'
import { CouponSelector } from '@/features/wallet/components/coupon-selector'
import {
  calculateCouponDiscount,
  getBalanceQuotaCost,
  isOnlinePayableAmount,
} from '@/features/wallet/lib/coupon'
import { useSystemConfig } from '@/hooks/use-system-config'
import { formatQuota } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { DEFAULT_CURRENCY_CONFIG } from '@/stores/system-config-store'

import {
  paySubscriptionStripe,
  paySubscriptionCreem,
  paySubscriptionEpay,
  paySubscriptionWaffoPancake,
  paySubscriptionBalance,
} from '../../api'
import {
  formatDuration,
  formatQuotaLimitWindow,
  formatResetPeriod,
  quotaLimitKey,
} from '../../lib'
import type { PlanRecord } from '../../types'

interface PaymentMethod {
  type: string
  name?: string
}

interface Props {
  open: boolean
  onOpenChange: (open: boolean) => void
  plan: PlanRecord | null
  enableStripe?: boolean
  enableCreem?: boolean
  enableWaffoPancake?: boolean
  enableOnlineTopUp?: boolean
  epayMethods?: PaymentMethod[]
  purchaseLimit?: number
  purchaseCount?: number
  /** The user already holds this plan: the purchase renews it instead of adding a slot. */
  renewal?: boolean
  userQuota?: number
  onPurchaseSuccess?: () => void | Promise<void>
}

export function SubscriptionPurchaseDialog(props: Props) {
  const { t } = useTranslation()
  const { currency } = useSystemConfig()
  const [paying, setPaying] = useState(false)
  const [selectedEpayMethod, setSelectedEpayMethod] = useState('')
  const [selectedCoupons, setSelectedCoupons] = useState<UserCoupon[]>([])
  const queryClient = useQueryClient()

  useEffect(() => {
    if (props.open && props.epayMethods && props.epayMethods.length > 0) {
      setSelectedEpayMethod(props.epayMethods[0].type)
    } else if (!props.open) {
      setSelectedEpayMethod('')
    }
  }, [props.open, props.epayMethods])

  const planId = props.plan?.plan?.id
  // Coupons are chosen per checkout: start empty whenever it opens or the plan changes.
  useEffect(() => {
    setSelectedCoupons([])
  }, [props.open, planId])

  const plan = props.plan?.plan
  if (!plan) return null

  const hasStripe = props.enableStripe && !!plan.stripe_price_id
  const hasCreem = props.enableCreem && !!plan.creem_product_id
  const hasWaffoPancake =
    props.enableWaffoPancake && !!plan.waffo_pancake_product_id
  const hasEpay =
    props.enableOnlineTopUp && (props.epayMethods || []).length > 0
  const hasAnyPayment = hasStripe || hasCreem || hasWaffoPancake || hasEpay
  const totalAmount = Number(plan.total_amount || 0)
  const priceAmount = Number(plan.price_amount || 0)
  const couponSummary = calculateCouponDiscount(priceAmount, selectedCoupons)
  const hasCoupons = selectedCoupons.length > 0
  const finalAmount = hasCoupons ? couponSummary.finalAmount : priceAmount
  const couponIds = hasCoupons
    ? selectedCoupons.map((coupon) => coupon.id)
    : undefined
  const onlineAmountTooLow = hasCoupons && !isOnlinePayableAmount(finalAmount)
  const price = priceAmount.toFixed(2)
  const quotaPerUnit =
    currency?.quotaPerUnit && currency.quotaPerUnit > 0
      ? currency.quotaPerUnit
      : DEFAULT_CURRENCY_CONFIG.quotaPerUnit
  const balanceCost = getBalanceQuotaCost(finalAmount, quotaPerUnit)
  const userQuota = Math.max(0, Number(props.userQuota || 0))
  const allowBalancePay = plan.allow_balance_pay !== false
  const insufficientBalance = userQuota < balanceCost
  const limitReached =
    !props.renewal &&
    (props.purchaseLimit || 0) > 0 &&
    (props.purchaseCount || 0) >= (props.purchaseLimit || 0)
  const planQuotaLimits = plan.quota_limits || []
  const legacyResetPeriod = formatResetPeriod(plan, t)
  const showLegacyReset =
    planQuotaLimits.length === 0 && legacyResetPeriod !== t('No Reset')

  const handlePayStripe = async () => {
    setPaying(true)
    try {
      const res = await paySubscriptionStripe({ plan_id: plan.id })
      if (res.message === 'success' && res.data?.pay_link) {
        window.open(res.data.pay_link, '_blank')
        toast.success(t('Payment page opened'))
        props.onOpenChange(false)
      } else {
        handleServerError(res, t('Payment request failed'))
      }
    } catch (error) {
      handleServerError(error, t('Payment request failed'))
    } finally {
      setPaying(false)
    }
  }

  const handlePayCreem = async () => {
    setPaying(true)
    try {
      const res = await paySubscriptionCreem({ plan_id: plan.id })
      if (res.message === 'success' && res.data?.checkout_url) {
        window.open(res.data.checkout_url, '_blank')
        toast.success(t('Payment page opened'))
        props.onOpenChange(false)
      } else {
        handleServerError(res, t('Payment request failed'))
      }
    } catch (error) {
      handleServerError(error, t('Payment request failed'))
    } finally {
      setPaying(false)
    }
  }

  // Coupons used by a new order are now reserved or consumed.
  const refreshCoupons = () => {
    if (hasCoupons) {
      void queryClient.invalidateQueries({ queryKey: userCouponsQueryKey })
    }
  }

  // In-tab redirect (not window.open) — user-gesture context is lost
  // across the await, so a popup would be blocked. Same as the wallet hook.
  const handlePayWaffoPancake = async () => {
    setPaying(true)
    try {
      const res = await paySubscriptionWaffoPancake({
        plan_id: plan.id,
        coupon_ids: couponIds,
      })
      if (res.message === 'success' && res.data?.checkout_url) {
        refreshCoupons()
        toast.success(t('Redirecting to payment page...'))
        window.location.href = res.data.checkout_url
      } else {
        handleServerError(res, t('Payment request failed'))
      }
    } catch (error) {
      handleServerError(error, t('Payment request failed'))
    } finally {
      setPaying(false)
    }
  }

  const isSafari =
    typeof navigator !== 'undefined' &&
    /^((?!chrome|android).)*safari/i.test(navigator.userAgent)

  const handlePayEpay = async () => {
    if (!selectedEpayMethod) {
      toast.error(t('Please select a payment method'))
      return
    }
    setPaying(true)
    try {
      const res = await paySubscriptionEpay({
        plan_id: plan.id,
        payment_method: selectedEpayMethod,
        coupon_ids: couponIds,
      })
      if (res.message === 'success' && res.url) {
        refreshCoupons()
        const form = document.createElement('form')
        form.action = res.url
        form.method = 'POST'
        if (!isSafari) {
          form.target = '_blank'
        }
        Object.entries(res.data || {}).forEach(([key, value]) => {
          const input = document.createElement('input')
          input.type = 'hidden'
          input.name = key
          input.value = String(value)
          form.appendChild(input)
        })
        document.body.appendChild(form)
        form.submit()
        document.body.removeChild(form)
        toast.success(t('Payment initiated'))
        props.onOpenChange(false)
      } else {
        handleServerError(res, t('Payment request failed'))
      }
    } catch (error) {
      handleServerError(error, t('Payment request failed'))
    } finally {
      setPaying(false)
    }
  }

  const handlePayBalance = async () => {
    if (!allowBalancePay) {
      toast.error(t('This plan does not allow balance redemption'))
      return
    }
    setPaying(true)
    try {
      const res = await paySubscriptionBalance({
        plan_id: plan.id,
        coupon_ids: couponIds,
      })
      if (res.success) {
        refreshCoupons()
        toast.success(t('Subscription purchased successfully'))
        void props.onPurchaseSuccess?.()
        props.onOpenChange(false)
      } else {
        handleServerError(res, t('Payment request failed'))
      }
    } catch (error) {
      handleServerError(error, t('Payment request failed'))
    } finally {
      setPaying(false)
    }
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={props.onOpenChange}
      title={
        <>
          <Crown className='h-5 w-5' />
          {t('Purchase Subscription')}
        </>
      }
      contentClassName='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md'
      titleClassName='flex items-center gap-2'
      contentHeight='auto'
      bodyClassName='space-y-4'
    >
      <div className='space-y-3 sm:space-y-4'>
        <div className='bg-muted/50 space-y-2.5 rounded-lg border p-3 sm:space-y-3 sm:p-4'>
          <div className='flex justify-between'>
            <span className='text-muted-foreground text-sm'>
              {t('Plan Name')}
            </span>
            <span className='max-w-[200px] truncate text-sm font-medium'>
              {plan.title}
            </span>
          </div>
          <div className='flex items-center justify-between'>
            <span className='text-muted-foreground text-sm'>
              {t('Validity Period')}
            </span>
            <span className='flex items-center gap-1 text-sm'>
              <CalendarClock className='h-3.5 w-3.5' />
              {formatDuration(plan, t)}
            </span>
          </div>
          {planQuotaLimits.length > 0 && (
            <div className='flex justify-between gap-3'>
              <span className='text-muted-foreground text-sm'>
                {t('Quota Limits')}
              </span>
              <div className='flex flex-col items-end gap-0.5 text-sm'>
                {planQuotaLimits.map((limit) => (
                  <span key={quotaLimitKey(limit)}>
                    {formatQuotaLimitWindow(limit, t)} ·{' '}
                    {Number(limit.amount || 0) > 0
                      ? formatQuota(Number(limit.amount || 0))
                      : t('Unlimited')}
                  </span>
                ))}
              </div>
            </div>
          )}
          {showLegacyReset && (
            <div className='flex justify-between'>
              <span className='text-muted-foreground text-sm'>
                {t('Reset Period')}
              </span>
              <span className='text-sm'>{legacyResetPeriod}</span>
            </div>
          )}
          <div className='flex items-center justify-between'>
            <span className='text-muted-foreground text-sm'>
              {t('Plan Quota')}
            </span>
            <span className='flex items-center gap-1 text-sm'>
              <Package className='h-3.5 w-3.5' />
              {totalAmount > 0 ? formatQuota(totalAmount) : t('Unlimited')}
            </span>
          </div>
          {plan.upgrade_group && (
            <div className='flex items-center justify-between'>
              <span className='text-muted-foreground text-sm'>
                {t('Upgrade Group')}
              </span>
              <GroupBadge group={plan.upgrade_group} />
            </div>
          )}
          <Separator />
          {hasCoupons && (
            <>
              <div className='flex items-center justify-between'>
                <span className='text-muted-foreground text-sm'>
                  {t('Original price')}
                </span>
                <span className='text-sm tabular-nums'>${price}</span>
              </div>
              <div className='flex items-center justify-between'>
                <span className='text-muted-foreground text-sm'>
                  {t('Coupon discount')}
                </span>
                <span className='text-sm tabular-nums'>
                  -${couponSummary.appliedDiscount.toFixed(2)}
                </span>
              </div>
            </>
          )}
          <div className='flex items-center justify-between'>
            <span className='text-sm font-medium'>{t('Amount Due')}</span>
            <span className='text-primary text-lg font-bold'>
              ${finalAmount.toFixed(2)}
            </span>
          </div>
        </div>

        <CouponSelector
          purchase='subscription'
          orderAmount={priceAmount}
          selected={selectedCoupons}
          onChange={setSelectedCoupons}
          enabled={props.open}
          disabled={paying || limitReached}
        />

        {limitReached && (
          <Alert variant='destructive'>
            <AlertDescription>
              {t('Purchase limit reached')} ({props.purchaseCount}/
              {props.purchaseLimit})
            </AlertDescription>
          </Alert>
        )}

        {props.renewal && !limitReached && (
          <Alert>
            <AlertDescription>
              {t(
                'This purchase renews your active subscription, resets its quota usage and extends its validity.'
              )}
            </AlertDescription>
          </Alert>
        )}

        <div className='flex flex-col gap-2 rounded-md border p-3'>
          <div className='flex items-center justify-between gap-2 text-xs'>
            <span className='text-muted-foreground'>{t('Required')}</span>
            <span>{formatQuota(balanceCost)}</span>
          </div>
          <div className='flex items-center justify-between gap-2 text-xs'>
            <span className='text-muted-foreground'>{t('Available')}</span>
            <span>{formatQuota(userQuota)}</span>
          </div>
          {!allowBalancePay ? (
            <Alert variant='destructive'>
              <AlertDescription>
                {t('This plan does not allow balance redemption')}
              </AlertDescription>
            </Alert>
          ) : (
            insufficientBalance && (
              <Alert variant='destructive'>
                <AlertDescription>{t('Insufficient balance')}</AlertDescription>
              </Alert>
            )
          )}
          <Button
            variant='outline'
            onClick={handlePayBalance}
            disabled={
              paying || limitReached || !allowBalancePay || insufficientBalance
            }
          >
            {t('Pay with Balance')}
          </Button>
        </div>

        {hasAnyPayment && (
          <div className='space-y-3'>
            <p className='text-muted-foreground text-xs'>
              {t('Select payment method')}
            </p>
            {onlineAmountTooLow && (
              <p className='text-destructive text-xs'>
                {t('Payment amount after coupons is too low')}
              </p>
            )}
            {(hasStripe || hasCreem || hasWaffoPancake) && (
              <div className='grid grid-cols-2 gap-2 sm:flex'>
                {hasStripe && (
                  <Button
                    variant='outline'
                    className='flex-1'
                    onClick={handlePayStripe}
                    disabled={paying || limitReached || hasCoupons}
                  >
                    Stripe
                  </Button>
                )}
                {hasCreem && (
                  <Button
                    variant='outline'
                    className='flex-1'
                    onClick={handlePayCreem}
                    disabled={paying || limitReached || hasCoupons}
                  >
                    Creem
                  </Button>
                )}
                {hasWaffoPancake && (
                  <Button
                    variant='outline'
                    className='flex-1'
                    onClick={handlePayWaffoPancake}
                    disabled={paying || limitReached || onlineAmountTooLow}
                  >
                    Waffo Pancake
                  </Button>
                )}
              </div>
            )}
            {hasCoupons && (hasStripe || hasCreem) && (
              <p className='text-muted-foreground text-xs'>
                {t('Coupons cannot be used with Stripe or Creem')}
              </p>
            )}
            {hasEpay && (
              <div className='grid grid-cols-[minmax(0,1fr)_auto] gap-2'>
                <Combobox
                  options={(props.epayMethods || []).map((m) => ({
                    value: m.type,
                    label: m.name || m.type,
                  }))}
                  value={selectedEpayMethod}
                  onValueChange={(v) => v !== null && setSelectedEpayMethod(v)}
                  disabled={limitReached}
                  className='flex-1'
                />
                <Button
                  onClick={handlePayEpay}
                  disabled={
                    paying ||
                    !selectedEpayMethod ||
                    limitReached ||
                    onlineAmountTooLow
                  }
                >
                  {t('Pay')}
                </Button>
              </div>
            )}
          </div>
        )}
      </div>
    </Dialog>
  )
}
