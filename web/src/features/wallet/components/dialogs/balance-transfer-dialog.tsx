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
import { Loader2, Send } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  formatQuota,
  parseQuotaFromDollars,
  quotaUnitsToDollars,
} from '@/lib/format'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

interface BalanceTransferDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: (targetUserId: number, quota: number) => Promise<boolean>
  availableQuota: number
  transferring: boolean
}

export function BalanceTransferDialog({
  open,
  onOpenChange,
  onConfirm,
  availableQuota,
  transferring,
}: BalanceTransferDialogProps) {
  const { t } = useTranslation()
  const currencyConfig = useSystemConfigStore((state) => state.config.currency)
  const minimumQuota = Math.ceil(
    currencyConfig.quotaPerUnit > 0
      ? currencyConfig.quotaPerUnit
      : DEFAULT_CURRENCY_CONFIG.quotaPerUnit
  )
  const minimumAmount = quotaUnitsToDollars(minimumQuota)
  const maximumAmount = quotaUnitsToDollars(availableQuota)
  const [targetUserId, setTargetUserId] = useState('')
  const [amount, setAmount] = useState(minimumAmount)
  const parsedTargetUserId = Number(targetUserId)
  const hasValidTarget =
    Number.isInteger(parsedTargetUserId) && parsedTargetUserId > 0
  const transferQuota = parseQuotaFromDollars(amount)
  const belowMinimum = !Number.isFinite(amount) || transferQuota < minimumQuota
  const exceedsBalance = transferQuota > availableQuota
  const canTransfer = hasValidTarget && !belowMinimum && !exceedsBalance

  useEffect(() => {
    if (open) {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setTargetUserId('')
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setAmount(minimumAmount)
    }
  }, [minimumAmount, open])

  const handleConfirm = async () => {
    if (!canTransfer) return

    const success = await onConfirm(parsedTargetUserId, transferQuota)
    if (success) {
      onOpenChange(false)
    }
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={t('Transfer Balance')}
      description={t('Send balance to another user by their user ID.')}
      contentClassName='max-sm:w-[calc(100vw-1.5rem)] sm:max-w-md'
      titleClassName='text-xl font-semibold'
      footerClassName='grid grid-cols-2 gap-2 sm:flex'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={transferring}
          >
            {t('Cancel')}
          </Button>
          <Button
            onClick={handleConfirm}
            disabled={transferring || !canTransfer}
          >
            {transferring ? (
              <Loader2 className='mr-2 h-4 w-4 animate-spin' />
            ) : (
              <Send className='mr-2 h-4 w-4' />
            )}
            {t('Transfer Balance')}
          </Button>
        </>
      }
    >
      <div className='space-y-4 py-3 sm:space-y-6 sm:py-4'>
        <div className='space-y-2'>
          <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
            {t('Available Balance')}
          </Label>
          <div className='text-2xl font-semibold'>
            {formatQuota(availableQuota)}
          </div>
        </div>

        <div className='space-y-2'>
          <Label
            htmlFor='balance-transfer-target-user-id'
            className='text-muted-foreground text-xs font-medium tracking-wider uppercase'
          >
            {t('Recipient User ID')}
          </Label>
          <Input
            id='balance-transfer-target-user-id'
            inputMode='numeric'
            autoComplete='off'
            value={targetUserId}
            onChange={(event) =>
              setTargetUserId(event.target.value.replaceAll(/\D/g, ''))
            }
            placeholder={t("Enter the recipient's user ID")}
            className='font-mono'
          />
        </div>

        <div className='space-y-3'>
          <Label
            htmlFor='balance-transfer-amount'
            className='text-muted-foreground text-xs font-medium tracking-wider uppercase'
          >
            {t('Transfer Amount')}
          </Label>
          <Input
            id='balance-transfer-amount'
            type='number'
            value={amount}
            onChange={(event) => setAmount(Number(event.target.value))}
            min={minimumAmount}
            max={maximumAmount}
            step={minimumAmount}
            className='font-mono text-lg'
          />
          <p className='text-muted-foreground text-xs'>
            {t('Transfer amount must be at least {{amount}}', {
              amount: formatQuota(minimumQuota),
            })}
          </p>
          {exceedsBalance ? (
            <p className='text-xs text-red-600 dark:text-red-400'>
              {t('Transfer amount exceeds your available balance')}
            </p>
          ) : null}
        </div>

        <p className='text-muted-foreground text-xs'>
          {t('Transfers are final and cannot be reversed.')}
        </p>
      </div>
    </Dialog>
  )
}
