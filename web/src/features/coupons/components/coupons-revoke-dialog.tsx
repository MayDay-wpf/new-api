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
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { handleServerError } from '@/lib/handle-server-error'
import { createServerError } from '@/lib/server-error-message'

import { revokeCoupon } from '../api'
import { ERROR_MESSAGES } from '../constants'
import { couponsQueryKey } from '../query-keys'
import { useCoupons } from './coupons-provider'

export function CouponsRevokeDialog() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { open, setOpen, currentRow } = useCoupons()

  const mutation = useMutation({
    mutationFn: async (id: number) => {
      const result = await revokeCoupon(id)
      if (!result.success) {
        throw createServerError(result, t(ERROR_MESSAGES.REVOKE_FAILED))
      }
      return result.data?.revoked_count ?? 0
    },
    onSuccess: (count) => {
      toast.success(
        t('Coupon revoked. {{count}} unused coupons invalidated.', { count })
      )
      setOpen(null)
      void queryClient.invalidateQueries({ queryKey: couponsQueryKey })
    },
    onError: (error) => {
      handleServerError(error, t(ERROR_MESSAGES.REVOKE_FAILED))
    },
  })

  return (
    <ConfirmDialog
      destructive
      open={open === 'revoke'}
      onOpenChange={(isOpen) => !isOpen && setOpen(null)}
      handleConfirm={() => {
        if (currentRow) mutation.mutate(currentRow.id)
      }}
      isLoading={mutation.isPending}
      className='max-w-md'
      title={t('Revoke coupon?')}
      desc={t(
        'All unused coupons of {{name}} will become invalid. Used coupons are not affected.',
        { name: currentRow?.name ?? '' }
      )}
      confirmText={t('Revoke coupon')}
    />
  )
}
