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
import { Ban, Users } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { DataTableRowActionMenu } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import {
  DropdownMenuItem,
  DropdownMenuShortcut,
} from '@/components/ui/dropdown-menu'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { COUPON_STATUS } from '../constants'
import type { Coupon } from '../types'
import { useCoupons } from './coupons-provider'

interface DataTableRowActionsProps {
  coupon: Coupon
}

export function DataTableRowActions(props: DataTableRowActionsProps) {
  const { t } = useTranslation()
  const { setOpen, setCurrentRow } = useCoupons()
  const canRevoke = props.coupon.status === COUPON_STATUS.ACTIVE

  return (
    <div className='-ml-1.5 flex items-center gap-1'>
      <Tooltip>
        <TooltipTrigger
          render={
            <Button
              variant='ghost'
              size='icon-sm'
              onClick={() => {
                setCurrentRow(props.coupon)
                setOpen('recipients')
              }}
              aria-label={t('View recipients')}
            />
          }
        >
          <Users />
        </TooltipTrigger>
        <TooltipContent>{t('View recipients')}</TooltipContent>
      </Tooltip>

      <DataTableRowActionMenu ariaLabel={t('Open menu')} modal={false}>
        <DropdownMenuItem
          disabled={!canRevoke}
          onClick={() => {
            setCurrentRow(props.coupon)
            setOpen('revoke')
          }}
          className='text-destructive focus:text-destructive'
        >
          {t('Revoke coupon')}
          <DropdownMenuShortcut>
            <Ban size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
      </DataTableRowActionMenu>
    </div>
  )
}
