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
import React, { useState } from 'react'

import useDialogState from '@/hooks/use-dialog'

import type { Coupon, CouponsDialogType } from '../types'

type CouponsContextType = {
  open: CouponsDialogType | null
  setOpen: (str: CouponsDialogType | null) => void
  currentRow: Coupon | null
  setCurrentRow: React.Dispatch<React.SetStateAction<Coupon | null>>
}

const CouponsContext = React.createContext<CouponsContextType | null>(null)

export function CouponsProvider(props: { children: React.ReactNode }) {
  const [open, setOpen] = useDialogState<CouponsDialogType>(null)
  const [currentRow, setCurrentRow] = useState<Coupon | null>(null)

  return (
    <CouponsContext value={{ open, setOpen, currentRow, setCurrentRow }}>
      {props.children}
    </CouponsContext>
  )
}

// eslint-disable-next-line react-refresh/only-export-components
export const useCoupons = () => {
  const couponsContext = React.useContext(CouponsContext)

  if (!couponsContext) {
    throw new Error('useCoupons has to be used within <CouponsProvider>')
  }

  return couponsContext
}
