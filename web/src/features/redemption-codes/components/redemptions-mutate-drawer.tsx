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
import { zodResolver } from '@hookform/resolvers/zod'
import { type FormEvent, useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DateTimePicker } from '@/components/datetime-picker'
import {
  SideDrawerSection,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
} from '@/components/drawer-layout'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Combobox } from '@/components/ui/combobox'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Switch } from '@/components/ui/switch'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { getAdminPlans } from '@/features/subscriptions/api'
import {
  formatQuotaLimitWindow,
  quotaLimitKey,
} from '@/features/subscriptions/lib'
import type { PlanRecord } from '@/features/subscriptions/types'
import {
  formatQuotaWithCurrency,
  getCurrencyDisplay,
  getCurrencyLabel,
} from '@/lib/currency'
import {
  formatQuota,
  getEditableQuotaStep,
  parseQuotaFromDollars,
} from '@/lib/format'
import { handleServerError } from '@/lib/handle-server-error'
import { addTimeToDate } from '@/lib/time'

import { createRedemption, updateRedemption, getRedemption } from '../api'
import { SUCCESS_MESSAGES } from '../constants'
import {
  getRedemptionFormSchema,
  type RedemptionFormValues,
  REDEMPTION_FORM_DEFAULT_VALUES,
  transformFormDataToPayload,
  transformRedemptionToFormDefaults,
} from '../lib'
import type { Redemption } from '../types'
import {
  RedemptionsExportDialog,
  type RedemptionExportData,
} from './redemptions-export-dialog'
import { useRedemptions } from './redemptions-provider'

type RedemptionsMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: Redemption
}

export function RedemptionsMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: RedemptionsMutateDrawerProps) {
  const { t } = useTranslation()
  const isUpdate = !!currentRow
  const redemptionId = currentRow?.id
  const { triggerRefresh } = useRedemptions()
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [createdCodes, setCreatedCodes] = useState<RedemptionExportData | null>(
    null
  )
  const [redemptionLoadState, setRedemptionLoadState] = useState<
    'idle' | 'loading' | 'ready' | 'error'
  >('idle')
  const [loadedRedemption, setLoadedRedemption] = useState<Redemption | null>(
    null
  )

  const form = useForm<RedemptionFormValues>({
    resolver: zodResolver(getRedemptionFormSchema(t)),
    defaultValues: REDEMPTION_FORM_DEFAULT_VALUES,
  })
  const redemptionType = form.watch('type')
  const isSubscriptionType = redemptionType === 'subscription'
  const isResetType = redemptionType === 'reset'
  const needsPlans = isSubscriptionType || isResetType
  const [plans, setPlans] = useState<PlanRecord[]>([])
  const [plansLoaded, setPlansLoaded] = useState(false)

  // Load subscription plans lazily, only when a subscription/reset code is being edited/created
  useEffect(() => {
    if (!open || !needsPlans || plansLoaded) return
    let ignoreResult = false
    void getAdminPlans()
      .then((result) => {
        if (ignoreResult) return
        if (result.success) {
          setPlans(result.data || [])
          setPlansLoaded(true)
        } else {
          handleServerError(result)
        }
      })
      .catch((error: unknown) => {
        if (!ignoreResult) handleServerError(error)
      })
    return () => {
      ignoreResult = true
    }
  }, [open, needsPlans, plansLoaded])

  useEffect(() => {
    if (!open) setPlansLoaded(false)
  }, [open])

  const getPlanTitle = (planId: number) =>
    plans.find((p) => p.plan.id === planId)?.plan.title || `#${planId}`

  const resetPlanId = form.watch('reset_plan_id')
  const resetPlanLimits = useMemo(
    () =>
      plans.find((p) => p.plan.id === resetPlanId)?.plan.quota_limits ?? [],
    [plans, resetPlanId]
  )

  const handleResetPlanChange = (planId: number) => {
    form.setValue('reset_plan_id', planId, { shouldValidate: true })
    const limits =
      plans.find((p) => p.plan.id === planId)?.plan.quota_limits ?? []
    form.setValue(
      'reset_quota_limit_keys',
      limits.map((limit) => quotaLimitKey(limit)),
      { shouldValidate: true }
    )
  }

  // Load existing data when updating
  useEffect(() => {
    if (!open) {
      form.reset(REDEMPTION_FORM_DEFAULT_VALUES)
      setRedemptionLoadState('idle')
      setLoadedRedemption(null)
      return
    }

    if (!isUpdate || redemptionId === undefined) {
      form.reset(REDEMPTION_FORM_DEFAULT_VALUES)
      setRedemptionLoadState('ready')
      setLoadedRedemption(null)
      return
    }

    let ignoreResult = false

    form.reset(REDEMPTION_FORM_DEFAULT_VALUES)
    setRedemptionLoadState('loading')
    setLoadedRedemption(null)

    void getRedemption(redemptionId)
      .then((result) => {
        if (ignoreResult) return

        if (
          !result.success ||
          !result.data ||
          result.data.id !== redemptionId
        ) {
          setRedemptionLoadState('error')
          handleServerError(result, t('Failed to load'))
          return
        }

        form.reset(transformRedemptionToFormDefaults(result.data))
        setLoadedRedemption(result.data)
        setRedemptionLoadState('ready')
      })
      .catch((error: unknown) => {
        if (ignoreResult) return

        setRedemptionLoadState('error')
        handleServerError(error)
      })

    return () => {
      ignoreResult = true
    }
  }, [open, isUpdate, redemptionId, form, t])

  const isUpdateReady =
    !isUpdate ||
    (redemptionLoadState === 'ready' && loadedRedemption?.id === redemptionId)
  const isLoadingRedemption = redemptionLoadState === 'loading'

  const onSubmit = async (data: RedemptionFormValues) => {
    if (isUpdate && (!currentRow || !loadedRedemption || !isUpdateReady)) {
      return
    }

    setIsSubmitting(true)
    try {
      const basePayload = transformFormDataToPayload(data)

      if (isUpdate && currentRow && loadedRedemption) {
        const keepLoadedQuota =
          data.type === 'quota' &&
          (loadedRedemption.plan_id ?? 0) <= 0 &&
          !form.getFieldState('quota_dollars').isDirty
        const quota = keepLoadedQuota
          ? loadedRedemption.quota
          : basePayload.quota
        const result = await updateRedemption({
          ...basePayload,
          quota,
          id: currentRow.id,
        })
        if (result.success) {
          toast.success(t(SUCCESS_MESSAGES.REDEMPTION_UPDATED))
          onOpenChange(false)
          triggerRefresh()
        } else {
          handleServerError(result)
        }
      } else {
        // Create mode
        const result = await createRedemption(basePayload)
        if (result.success) {
          const count = result.data?.length || 0
          toast.success(
            count > 1
              ? t('Successfully created {{count}} redemption codes', {
                  count,
                })
              : t(SUCCESS_MESSAGES.REDEMPTION_CREATED)
          )
          if (result.data?.length) {
            setCreatedCodes({
              keys: result.data,
              name: basePayload.name,
              quota:
                basePayload.plan_id > 0
                  ? getPlanTitle(basePayload.plan_id)
                  : formatQuotaWithCurrency(basePayload.quota, {
                      abbreviate: false,
                    }),
            })
          }
          onOpenChange(false)
          triggerRefresh()
        } else {
          handleServerError(result)
        }
      }
    } catch (error) {
      handleServerError(error)
    } finally {
      setIsSubmitting(false)
    }
  }

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    if (!isUpdate) {
      const name = form.getValues('name')
      if (!name?.trim()) {
        const type = form.getValues('type')
        if (type === 'subscription') {
          const planId = form.getValues('plan_id')
          if (planId > 0) {
            form.setValue('name', getPlanTitle(planId).slice(0, 20), {
              shouldValidate: true,
            })
          }
        } else if (type === 'reset') {
          const planId = form.getValues('reset_plan_id')
          if (planId > 0) {
            form.setValue('name', getPlanTitle(planId).slice(0, 20), {
              shouldValidate: true,
            })
          }
        } else {
          const quota = parseQuotaFromDollars(form.getValues('quota_dollars'))
          form.setValue('name', formatQuota(quota), { shouldValidate: true })
        }
      }
    }

    void form.handleSubmit(onSubmit)(event)
  }

  const handleSetExpiry = (months: number, days: number, hours: number) => {
    const newDate = addTimeToDate(months, days, hours)
    form.setValue('expired_time', newDate ?? null)
  }

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'
  const quotaStep = getEditableQuotaStep()
  const quotaLabel = t('Quota ({{currency}})', { currency: currencyLabel })
  const quotaPlaceholder = tokensOnly
    ? t('Enter quota in tokens')
    : t('Enter quota in {{currency}}', { currency: currencyLabel })
  let submitButtonLabel = t('Save changes')
  if (isLoadingRedemption) {
    submitButtonLabel = t('Loading...')
  } else if (isSubmitting) {
    submitButtonLabel = t('Saving...')
  }
  let typeDescription = t("Redeeming adds the quota to the user's balance.")
  if (isSubscriptionType) {
    typeDescription = t(
      'Redeeming grants the selected subscription plan. Each unredeemed code reserves one unit of plan stock.'
    )
  } else if (isResetType) {
    typeDescription = t(
      'Redeeming the reset card resets the selected quota limit windows of the user\'s active subscription for this plan.'
    )
  }

  return (
    <>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent
          className={sideDrawerContentClassName('sm:max-w-[600px]')}
        >
          <SheetHeader className={sideDrawerHeaderClassName()}>
            <SheetTitle>
              {isUpdate
                ? t('Update Redemption Code')
                : t('Create Redemption Code')}
            </SheetTitle>
            <SheetDescription>
              {isUpdate
                ? t('Update the redemption code by providing necessary info.')
                : t(
                    'Add new redemption code(s) by providing necessary info.'
                  )}{' '}
              {t('Click save when you&apos;re done.')}
            </SheetDescription>
          </SheetHeader>
          <Form {...form}>
            <form
              id='redemption-form'
              onSubmit={handleSubmit}
              className={sideDrawerFormClassName()}
              aria-busy={isLoadingRedemption}
            >
              <fieldset
                disabled={!isUpdateReady || isSubmitting}
                className='contents'
              >
                <SideDrawerSection>
                  <FormField
                    control={form.control}
                    name='type'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Redemption code type')}</FormLabel>
                        <FormControl>
                          <RadioGroup
                            value={field.value}
                            onValueChange={(value) => {
                              if (
                                value === 'quota' ||
                                value === 'subscription' ||
                                value === 'reset'
                              ) {
                                field.onChange(value)
                                form.setValue('plan_id', 0)
                                form.setValue('reset_plan_id', 0)
                                form.setValue('reset_quota_limit_keys', [])
                              }
                            }}
                            disabled={isUpdate}
                            className='flex flex-wrap gap-x-5 gap-y-3'
                          >
                            {[
                              { value: 'quota', label: t('Balance') },
                              {
                                value: 'subscription',
                                label: t('Subscription'),
                              },
                              {
                                value: 'reset',
                                label: t('Reset Card'),
                              },
                            ].map((option) => (
                              <div
                                key={option.value}
                                className='flex items-center gap-2'
                              >
                                <RadioGroupItem
                                  value={option.value}
                                  id={`redemption-type-${option.value}`}
                                />
                                <Label
                                  htmlFor={`redemption-type-${option.value}`}
                                >
                                  {option.label}
                                </Label>
                              </div>
                            ))}
                          </RadioGroup>
                        </FormControl>
                        <FormDescription>
                          {typeDescription}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='name'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Name')}</FormLabel>
                        <FormControl>
                          <Input {...field} placeholder={t('Enter a name')} />
                        </FormControl>
                        <FormDescription>
                          {t('Name for this redemption code (1-20 characters)')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  {isSubscriptionType && (
                    <FormField
                      control={form.control}
                      name='plan_id'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Subscription plan')}</FormLabel>
                          <FormControl>
                            <Combobox
                              options={plans.map((p) => {
                                const stockTotal = p.plan.stock_total ?? 0
                                const remaining = Math.max(
                                  stockTotal - (p.plan.stock_sold ?? 0),
                                  0
                                )
                                return {
                                  value: String(p.plan.id),
                                  label:
                                    stockTotal > 0
                                      ? `${p.plan.title} (${t('Remaining Stock')}: ${remaining})`
                                      : p.plan.title,
                                }
                              })}
                              value={field.value > 0 ? String(field.value) : ''}
                              onValueChange={(v) =>
                                v !== null && field.onChange(Number(v) || 0)
                              }
                              placeholder={t('Select subscription plan')}
                            />
                          </FormControl>
                          <FormDescription>
                            {t(
                              'Creation fails if the remaining plan stock is less than the number of codes.'
                            )}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  )}

                  {isResetType && (
                    <>
                      <FormField
                        control={form.control}
                        name='reset_plan_id'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Subscription plan')}</FormLabel>
                            <FormControl>
                              <Combobox
                                options={plans.map((p) => ({
                                  value: String(p.plan.id),
                                  label: p.plan.title,
                                }))}
                                value={
                                  field.value > 0 ? String(field.value) : ''
                                }
                                onValueChange={(v) =>
                                  v !== null &&
                                  handleResetPlanChange(Number(v) || 0)
                                }
                                placeholder={t('Select subscription plan')}
                              />
                            </FormControl>
                            <FormDescription>
                              {t(
                                'Only plans with configured quota limits can issue reset cards.'
                              )}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />

                      <FormField
                        control={form.control}
                        name='reset_quota_limit_keys'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Limits to reset')}</FormLabel>
                            {resetPlanLimits.length === 0 ? (
                              <p className='text-muted-foreground text-sm'>
                                {t(
                                  'This subscription plan has no resettable quota limits'
                                )}
                              </p>
                            ) : (
                              <div className='space-y-2'>
                                {resetPlanLimits.map((limit) => {
                                  const key = quotaLimitKey(limit)
                                  const checked = field.value.includes(key)
                                  return (
                                    <label
                                      key={key}
                                      className='flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-sm'
                                    >
                                      <span className='flex items-center gap-2'>
                                        <Checkbox
                                          checked={checked}
                                          onCheckedChange={(checkedValue) =>
                                            field.onChange(
                                              checkedValue
                                                ? [...field.value, key]
                                                : field.value.filter(
                                                    (item) => item !== key
                                                  )
                                            )
                                          }
                                        />
                                        <span>
                                          {formatQuotaLimitWindow(limit, t)}
                                        </span>
                                      </span>
                                      <span className='text-muted-foreground text-xs'>
                                        {Number(limit.amount || 0) > 0
                                          ? formatQuota(Number(limit.amount))
                                          : t('Unlimited')}
                                      </span>
                                    </label>
                                  )
                                })}
                              </div>
                            )}
                            <FormDescription>
                              {t(
                                'Select one or more quota windows. Only the selected windows are cleared when the card is redeemed.'
                              )}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />

                      <FormField
                        control={form.control}
                        name='reset_advance_time'
                        render={({ field }) => (
                          <FormItem>
                            <label className='flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-sm'>
                              <span>{t('Advance next reset time')}</span>
                              <Switch
                                checked={field.value}
                                onCheckedChange={(checked) =>
                                  field.onChange(!!checked)
                                }
                                aria-label={t('Advance next reset time')}
                              />
                            </label>
                            <FormDescription>
                              {t(
                                'When enabled, the next automatic reset is recalculated from the moment the card is redeemed.'
                              )}
                            </FormDescription>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    </>
                  )}

                  {!isSubscriptionType && !isResetType && (
                    <FormField
                      control={form.control}
                      name='quota_dollars'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{quotaLabel}</FormLabel>
                          <FormControl>
                            <Input
                              {...field}
                              type='number'
                              step={quotaStep}
                              placeholder={quotaPlaceholder}
                              onChange={(e) =>
                                field.onChange(
                                  Number.parseFloat(e.target.value) || 0
                                )
                              }
                            />
                          </FormControl>
                          <FormDescription>
                            {tokensOnly
                              ? t('Enter the quota amount in tokens')
                              : t('Enter the quota amount in {{currency}}', {
                                  currency: currencyLabel,
                                })}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  )}

                  <div className='grid gap-4 sm:grid-cols-2'>
                    <FormField
                      control={form.control}
                      name='max_uses'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Total usage limit')}</FormLabel>
                          <FormControl>
                            <Input
                              {...field}
                              type='number'
                              min='0'
                              step='1'
                              onChange={(e) =>
                                field.onChange(
                                  Math.max(
                                    0,
                                    Number.parseInt(e.target.value, 10) || 0
                                  )
                                )
                              }
                            />
                          </FormControl>
                          <FormDescription>
                            {isUpdate
                              ? t(
                                  'Already used {{count}} times. 0 means unlimited',
                                  { count: loadedRedemption?.used_count ?? 0 }
                                )
                              : t('0 means unlimited')}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    <FormField
                      control={form.control}
                      name='per_user_limit'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Per-account usage limit')}</FormLabel>
                          <FormControl>
                            <Input
                              {...field}
                              type='number'
                              min='0'
                              step='1'
                              onChange={(e) =>
                                field.onChange(
                                  Math.max(
                                    0,
                                    Number.parseInt(e.target.value, 10) || 0
                                  )
                                )
                              }
                            />
                          </FormControl>
                          <FormDescription>
                            {t('0 means unlimited')}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>

                  <FormField
                    control={form.control}
                    name='expired_time'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Expiration Time')}</FormLabel>
                        <div className='flex flex-col gap-2'>
                          <FormControl>
                            <DateTimePicker
                              value={field.value ?? undefined}
                              onChange={(date) => field.onChange(date ?? null)}
                              placeholder={t('Never expires')}
                            />
                          </FormControl>
                          <div className='grid grid-cols-4 gap-1.5 sm:flex sm:gap-2'>
                            <Button
                              type='button'
                              variant='outline'
                              size='sm'
                              onClick={() => handleSetExpiry(0, 0, 0)}
                            >
                              {t('Never')}
                            </Button>
                            <Button
                              type='button'
                              variant='outline'
                              size='sm'
                              onClick={() => handleSetExpiry(1, 0, 0)}
                            >
                              {t('1M')}
                            </Button>
                            <Button
                              type='button'
                              variant='outline'
                              size='sm'
                              onClick={() => handleSetExpiry(0, 7, 0)}
                            >
                              {t('1W')}
                            </Button>
                            <Button
                              type='button'
                              variant='outline'
                              size='sm'
                              onClick={() => handleSetExpiry(0, 1, 0)}
                            >
                              {t('1 Day')}
                            </Button>
                          </div>
                        </div>
                        <FormDescription>
                          {t('Leave empty for never expires')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  {!isUpdate && (
                    <FormField
                      control={form.control}
                      name='count'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Quantity')}</FormLabel>
                          <FormControl>
                            <Input
                              {...field}
                              type='number'
                              min='1'
                              max='100'
                              placeholder={t('Number of codes to create')}
                              onChange={(e) =>
                                field.onChange(
                                  Number.parseInt(e.target.value, 10) || 1
                                )
                              }
                            />
                          </FormControl>
                          <FormDescription>
                            {t(
                              'Create multiple redemption codes at once (1-100)'
                            )}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  )}
                </SideDrawerSection>
              </fieldset>
            </form>
          </Form>
          <SheetFooter className={sideDrawerFooterClassName()}>
            <SheetClose render={<Button variant='outline' />}>
              {t('Close')}
            </SheetClose>
            <Button
              form='redemption-form'
              type='submit'
              disabled={isSubmitting || !isUpdateReady}
            >
              {submitButtonLabel}
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>
      {createdCodes && (
        <RedemptionsExportDialog
          data={createdCodes}
          onClose={() => setCreatedCodes(null)}
        />
      )}
    </>
  )
}
