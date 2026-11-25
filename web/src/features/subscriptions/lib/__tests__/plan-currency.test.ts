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
import { describe, expect, test } from 'vitest'

import {
  formatPlanPrice,
  getPlanCurrencySymbol,
  normalizePlanCurrency,
} from '../format'

describe('plan currency', () => {
  test('normalizes a stored code to a three-letter ISO 4217 code', () => {
    expect(normalizePlanCurrency('cny')).toBe('CNY')
    expect(normalizePlanCurrency(' eur ')).toBe('EUR')
    // Any three-letter code round-trips; the backend validates the same shape.
    expect(normalizePlanCurrency('RMB')).toBe('RMB')
  })

  test('falls back to USD for missing or malformed codes', () => {
    expect(normalizePlanCurrency('')).toBe('USD')
    expect(normalizePlanCurrency(undefined)).toBe('USD')
    expect(normalizePlanCurrency(null)).toBe('USD')
    expect(normalizePlanCurrency('EURO')).toBe('USD')
    expect(normalizePlanCurrency('12')).toBe('USD')
  })

  test('keeps two decimals and uses the plan currency, not the site currency', () => {
    expect(formatPlanPrice(12, 'CNY')).toMatch(/12[.,]00/)
    expect(formatPlanPrice(12, 'CNY')).not.toBe(formatPlanPrice(12, 'USD'))
    expect(formatPlanPrice(0, 'CNY')).toMatch(/0[.,]00/)
  })

  test('exposes a symbol for the plan editor picker', () => {
    expect(getPlanCurrencySymbol('CNY')).not.toBe('')
    expect(getPlanCurrencySymbol('')).toBe(getPlanCurrencySymbol('USD'))
  })
})
