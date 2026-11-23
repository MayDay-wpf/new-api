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
import i18next from 'i18next'
import { useCallback, useState } from 'react'
import { toast } from 'sonner'

import { handleServerError } from '@/lib/handle-server-error'
import {
  getServerMessage,
  requireServerSuccess,
} from '@/lib/server-error-message'

import { transferBalance as transferBalanceRequest } from '../api'

// ============================================================================
// Balance Transfer Hook
// ============================================================================

export function useBalanceTransfer() {
  const [transferring, setTransferring] = useState(false)

  const transferBalance = useCallback(
    async (targetUserId: number, quota: number): Promise<boolean> => {
      try {
        setTransferring(true)
        const response = requireServerSuccess(
          await transferBalanceRequest({
            target_user_id: targetUserId,
            quota,
          })
        )
        toast.success(
          getServerMessage(response) || i18next.t('Transfer successful')
        )
        return true
      } catch (error) {
        handleServerError(error, i18next.t('Transfer failed'))
        return false
      } finally {
        setTransferring(false)
      }
    },
    []
  )

  return {
    transferring,
    transferBalance,
  }
}
