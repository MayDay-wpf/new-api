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
import type { Table } from '@tanstack/react-table'
import { PowerOff, Trash2 } from 'lucide-react'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { ConfirmDialog } from '@/components/confirm-dialog'
import { DataTableBulkActions as BulkActionsToolbar } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { AdminUserBatchAction } from '@/features/auth/secure-verification'
import { handleServerError } from '@/lib/handle-server-error'
import { AuthOperationError } from '@/lib/secure-verification'
import { useAuthStore } from '@/stores/auth-store'

import { batchManageUsers } from '../api'
import {
  ERROR_MESSAGES,
  USER_ROLE,
  USER_STATUS,
  isUserDeleted,
} from '../constants'
import type { User } from '../types'
import { useUsers } from './users-provider'

/** Matches the backend MaxAdminUserBatchSize. */
const MAX_BATCH_SIZE = 100

interface DataTableBulkActionsProps {
  table: Table<User>
}

interface PendingBatch {
  action: AdminUserBatchAction
  targets: User[]
  skipped: number
}

export function DataTableBulkActions({ table }: DataTableBulkActionsProps) {
  const { t } = useTranslation()
  const { triggerRefresh, requestVerification, verificationActive } = useUsers()
  const currentUser = useAuthStore((s) => s.auth.user)
  const [pending, setPending] = useState<PendingBatch | null>(null)
  const [isSubmitting, setIsSubmitting] = useState(false)
  const selectedUsers = table
    .getFilteredSelectedRowModel()
    .rows.map((row) => row.original)

  // Mirror the server rules so one ineligible row cannot reject the batch:
  // deleted rows, the root account, yourself and peers are left out.
  const isManageable = (user: User) => {
    if (isUserDeleted(user) || user.role === USER_ROLE.ROOT) return false
    if (!currentUser || user.id === currentUser.id) return false
    return currentUser.role === USER_ROLE.ROOT || currentUser.role > user.role
  }

  const openConfirm = (action: AdminUserBatchAction) => {
    const targets = selectedUsers.filter(
      (user) =>
        isManageable(user) &&
        (action === 'delete' || user.status !== USER_STATUS.DISABLED)
    )
    if (targets.length === 0) {
      toast.error(t('None of the selected users can be changed'))
      return
    }
    if (targets.length > MAX_BATCH_SIZE) {
      toast.error(
        t('Too many items in batch request, maximum is {{max}}', {
          max: MAX_BATCH_SIZE,
        })
      )
      return
    }
    setPending({
      action,
      targets,
      skipped: selectedUsers.length - targets.length,
    })
  }

  const handleConfirm = async () => {
    if (!pending || isSubmitting) return
    const { action, targets } = pending
    const ids = targets.map((user) => user.id)
    setIsSubmitting(true)
    try {
      const proof = await requestVerification({
        scope: 'admin.user.batch',
        context: { action, user_ids: ids },
        title:
          action === 'delete'
            ? t('Verify to delete users')
            : t('Verify to disable users'),
        description: t(
          'Confirm your identity before changing {{count}} accounts.',
          { count: ids.length }
        ),
      })
      if (!proof) return
      const result = await batchManageUsers(ids, action, proof.proof_token)
      if (!result.success) {
        handleServerError(
          result,
          action === 'delete'
            ? t('Failed to delete {{count}} users', { count: ids.length })
            : t('Failed to disable {{count}} users', { count: ids.length })
        )
        return
      }
      const succeeded = result.data?.succeeded ?? 0
      const failed = result.data?.failed_ids?.length ?? 0
      if (failed > 0) {
        toast.warning(
          t('{{succeeded}} users updated, {{failed}} failed', {
            succeeded,
            failed,
          })
        )
      } else {
        toast.success(
          action === 'delete'
            ? t('Deleted {{count}} users', { count: succeeded })
            : t('Disabled {{count}} users', { count: succeeded })
        )
      }
      table.resetRowSelection()
      setPending(null)
      triggerRefresh()
    } catch (error) {
      handleServerError(
        AuthOperationError.from(error),
        t(ERROR_MESSAGES.UNEXPECTED)
      )
    } finally {
      setIsSubmitting(false)
    }
  }

  const count = pending?.targets.length ?? 0
  const isDelete = pending?.action === 'delete'
  let confirmText = t('Disable')
  if (isDelete) confirmText = isSubmitting ? t('Deleting...') : t('Delete')

  return (
    <>
      <BulkActionsToolbar table={table} entityName='user'>
        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='outline'
                size='icon'
                className='size-8'
                disabled={isSubmitting}
                onClick={() => openConfirm('disable')}
                aria-label={t('Disable selected users')}
              />
            }
          >
            <PowerOff aria-hidden='true' />
          </TooltipTrigger>
          <TooltipContent>{t('Disable selected users')}</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger
            render={
              <Button
                variant='destructive'
                size='icon'
                className='size-8'
                disabled={isSubmitting}
                onClick={() => openConfirm('delete')}
                aria-label={t('Delete selected users')}
              />
            }
          >
            <Trash2 aria-hidden='true' />
          </TooltipTrigger>
          <TooltipContent>{t('Delete selected users')}</TooltipContent>
        </Tooltip>
      </BulkActionsToolbar>

      <ConfirmDialog
        open={pending !== null && !verificationActive}
        onOpenChange={(open) => {
          if (!open && !isSubmitting) setPending(null)
        }}
        destructive={isDelete}
        title={
          isDelete
            ? t('Delete {{count}} users?', { count })
            : t('Disable {{count}} users?', { count })
        }
        desc={
          <div className='space-y-2'>
            <p>
              {isDelete
                ? t(
                    'The selected accounts and their data will be permanently deleted. This action cannot be undone.'
                  )
                : t(
                    'The selected accounts will be signed out and can no longer sign in or call the API.'
                  )}
            </p>
            {pending && pending.skipped > 0 && (
              <p className='text-muted-foreground'>
                {t(
                  '{{count}} selected users are skipped because they cannot be changed.',
                  { count: pending.skipped }
                )}
              </p>
            )}
          </div>
        }
        confirmText={confirmText}
        isLoading={isSubmitting}
        handleConfirm={handleConfirm}
      />
    </>
  )
}
