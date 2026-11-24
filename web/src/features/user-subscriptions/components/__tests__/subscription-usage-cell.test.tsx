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
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, test } from 'vitest'

import type { AdminUserSubscriptionRow } from '../../types'
import { SubscriptionUsageCell } from '../user-subscription-cells'

function usageRow(): AdminUserSubscriptionRow {
  return {
    subscription: {
      id: 1,
      user_id: 2,
      plan_id: 3,
      status: 'active',
      start_time: 0,
      end_time: Math.floor(Date.now() / 1000) + 24 * 60 * 60,
      amount_total: 1000,
      amount_used: 250,
    },
    username: 'alice',
    display_name: 'Alice',
    email: 'alice@example.com',
    plan_title: 'Alpha plan',
    plan_subtitle: '',
  }
}

describe('SubscriptionUsageCell', () => {
  test('shows the usage progress popover when hovered', async () => {
    render(<SubscriptionUsageCell row={usageRow()} />)

    expect(screen.getByText(/Used/)).toBeVisible()
    expect(document.querySelector('[data-slot="progress-track"]')).toBeNull()

    const trigger = document.querySelector<HTMLElement>(
      '[data-slot="popover-trigger"]'
    )
    if (!trigger) {
      throw new Error('Expected the usage hover trigger to render')
    }

    fireEvent.mouseEnter(trigger)
    fireEvent.pointerEnter(trigger)

    await waitFor(() => expect(screen.getByText('Usage')).toBeVisible())

    const popover = document.querySelector('[data-slot="popover-content"]')
    expect(popover).not.toBeNull()
    expect(
      popover?.querySelector('[data-slot="progress-track"]')
    ).toBeInTheDocument()
    expect(popover?.textContent).toContain('25%')
    expect(popover?.textContent).toContain('Used amount')
    expect(popover?.textContent).toContain('Remaining')
    expect(popover?.textContent).toContain('Total')
  })

  test('still shows a hover popover without any quota', async () => {
    const row = usageRow()
    row.subscription.amount_total = 0
    render(<SubscriptionUsageCell row={row} />)

    expect(screen.getByText('Unlimited')).toBeVisible()

    const trigger = document.querySelector<HTMLElement>(
      '[data-slot="popover-trigger"]'
    )
    if (!trigger) {
      throw new Error('Expected the unlimited usage hover trigger to render')
    }

    fireEvent.mouseEnter(trigger)

    await waitFor(() => expect(screen.getByText('Usage')).toBeVisible())

    const popover = document.querySelector('[data-slot="popover-content"]')
    expect(popover).not.toBeNull()
    expect(popover?.textContent).toContain('Unlimited')
  })

  test('shows periodic quota usage when the total quota is unlimited', async () => {
    const row = usageRow()
    const now = Math.floor(Date.now() / 1000)
    row.subscription.amount_total = 0
    row.subscription.amount_used = 0
    row.subscription.quota_limit_usage = [
      {
        key: 'custom:18000',
        used: 250,
        window_start: now - 100,
        window_end: now + 17900,
      },
    ]
    row.plan_quota_limits = [
      { period: 'custom', custom_seconds: 18000, amount: 1000 },
    ]

    render(<SubscriptionUsageCell row={row} />)

    expect(screen.getByText('5 hours')).toBeVisible()

    const trigger = document.querySelector<HTMLElement>(
      '[data-slot="popover-trigger"]'
    )
    if (!trigger) {
      throw new Error('Expected the periodic quota hover trigger to render')
    }

    fireEvent.mouseEnter(trigger)

    await waitFor(() => expect(screen.getByText('Usage')).toBeVisible())

    const popover = document.querySelector('[data-slot="popover-content"]')
    expect(popover).not.toBeNull()
    expect(popover?.textContent).toContain('Quota Limits')
    expect(popover?.textContent).toContain('25%')
    expect(
      popover?.querySelector('[data-slot="progress-track"]')
    ).toBeInTheDocument()
  })
})
