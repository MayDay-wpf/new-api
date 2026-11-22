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
import { Loader2, Ticket } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { TitledCard } from '@/components/ui/titled-card'
import {
  previewSubscriptionResetCard,
  redeemSubscriptionResetCard,
} from '@/features/subscriptions/api'
import { formatQuotaLimitWindow } from '@/features/subscriptions/lib'
import type { ResetCardPreview } from '@/features/subscriptions/types'
import { formatQuota } from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'

interface ResetCardRedeemCardProps {
  onRedeemed?: () => void | Promise<void>
}

export function ResetCardRedeemCard(props: ResetCardRedeemCardProps) {
  const { t } = useTranslation()
  const [code, setCode] = useState('')
  const [preview, setPreview] = useState<ResetCardPreview | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [previewing, setPreviewing] = useState(false)
  const [redeeming, setRedeeming] = useState(false)

  const handlePreview = async () => {
    const trimmed = code.trim()
    if (!trimmed) {
      toast.error(t('Please enter a redemption code'))
      return
    }
    setPreviewing(true)
    try {
      const res = await previewSubscriptionResetCard(trimmed)
      if (!res.success || !res.data) {
        handleServerError(res, t('Redemption failed'))
        return
      }
      if (!res.data.eligible) {
        toast.warning(
          t(
            'You have no active subscription for this plan. This reset card cannot be used and will not be consumed.'
          )
        )
        return
      }
      setPreview(res.data)
      setConfirmOpen(true)
    } catch (error) {
      handleServerError(error, t('Redemption failed'))
    } finally {
      setPreviewing(false)
    }
  }

  const handleConfirm = async () => {
    const trimmed = code.trim()
    if (!trimmed) return
    setRedeeming(true)
    try {
      const res = await redeemSubscriptionResetCard(trimmed)
      if (res.success && res.data) {
        toast.success(
          t('Reset card redeemed. Reset {{count}} subscription(s) of {{plan}}.', {
            count: res.data.reset_count,
            plan: res.data.reset_plan_title,
          })
        )
        setConfirmOpen(false)
        setPreview(null)
        setCode('')
        await props.onRedeemed?.()
      } else {
        handleServerError(res, t('Redemption failed'))
      }
    } catch (error) {
      handleServerError(error, t('Redemption failed'))
    } finally {
      setRedeeming(false)
    }
  }

  return (
    <>
      <TitledCard
        title={t('Subscription Reset Card')}
        description={t(
          'Redeem a reset card to clear quota usage of your active subscription'
        )}
        icon={<Ticket className='h-4 w-4' />}
        iconTone='info'
        disableHoverEffect
        contentClassName='space-y-3'
      >
        <div className='flex flex-col gap-2 sm:flex-row'>
          <Input
            value={code}
            onChange={(event) => setCode(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Enter') {
                event.preventDefault()
                void handlePreview()
              }
            }}
            placeholder={t('Enter reset card code')}
            className='flex-1'
          />
          <Button
            onClick={() => void handlePreview()}
            disabled={previewing || !code.trim()}
          >
            {previewing && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            {t('Redeem')}
          </Button>
        </div>
        <p className='text-muted-foreground text-xs'>
          {t(
            'A reset card only clears quota usage. It does not extend the subscription validity period.'
          )}
        </p>
      </TitledCard>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title={t('Confirm reset card redemption')}
        desc={
          <div className='space-y-3'>
            <p>
              {t(
                'The selected quota usage of {{plan}} will be cleared immediately. This only resets quota usage and does not extend the subscription validity period.',
                { plan: preview?.plan_title || '' }
              )}
            </p>
            <div className='space-y-2 rounded-md border p-3 text-sm'>
              <div className='flex items-center justify-between gap-3'>
                <span className='text-muted-foreground'>
                  {t('Subscription plan')}
                </span>
                <span className='font-medium'>{preview?.plan_title}</span>
              </div>
              <div className='space-y-1'>
                <span className='text-muted-foreground'>
                  {t('Limits to reset')}
                </span>
                <ul className='list-inside list-disc'>
                  {(preview?.limits || []).map((limit) => (
                    <li key={limit.key}>
                      {formatQuotaLimitWindow(limit, t)}
                      {' · '}
                      {Number(limit.amount || 0) > 0
                        ? formatQuota(Number(limit.amount))
                        : t('Unlimited')}
                    </li>
                  ))}
                </ul>
              </div>
              <div className='flex items-center justify-between gap-3'>
                <span className='text-muted-foreground'>
                  {t('Advance next reset time')}
                </span>
                <span>{preview?.advance_reset_time ? t('Yes') : t('No')}</span>
              </div>
              <div className='flex items-center justify-between gap-3'>
                <span className='text-muted-foreground'>
                  {t('Matching active subscriptions')}
                </span>
                <span>{preview?.subscription_count ?? 0}</span>
              </div>
            </div>
            <p className='text-muted-foreground text-xs'>
              {t('This action cannot be undone.')}
            </p>
          </div>
        }
        confirmText={t('Reset quota')}
        handleConfirm={() => void handleConfirm()}
        isLoading={redeeming}
      />
    </>
  )
}
