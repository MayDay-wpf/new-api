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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo } from 'react'
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
import { MultiSelect } from '@/components/multi-select'
import { Button } from '@/components/ui/button'
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
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { getGroups } from '@/features/users/api'
import { handleServerError } from '@/lib/handle-server-error'
import {
  createServerError,
  requireServerSuccess,
} from '@/lib/server-error-message'

import { createCoupon } from '../api'
import {
  COUPON_SCOPE_LABELS,
  COUPON_SCOPE_OPTIONS,
  ERROR_MESSAGES,
} from '../constants'
import {
  COUPON_FORM_DEFAULT_VALUES,
  type CouponFormValues,
  getCouponFormSchema,
  transformCouponFormToPayload,
} from '../lib'
import { couponsQueryKey } from '../query-keys'
import type { CouponScope, CouponTargetType } from '../types'

type CouponsMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const TARGET_OPTIONS: { value: CouponTargetType; labelKey: string }[] = [
  { value: 'all', labelKey: 'All users' },
  { value: 'custom', labelKey: 'Specified recipients' },
]

function parseAmountInput(value: string): number {
  const parsed = Number.parseFloat(value)
  return Number.isFinite(parsed) ? parsed : Number.NaN
}

export function CouponsMutateDrawer(props: CouponsMutateDrawerProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const schema = useMemo(() => getCouponFormSchema(t), [t])
  const form = useForm<CouponFormValues>({
    resolver: zodResolver(schema),
    defaultValues: COUPON_FORM_DEFAULT_VALUES,
  })
  const neverExpires = form.watch('never_expires')
  const targetType = form.watch('target_type')

  const { data: groupsData } = useQuery({
    queryKey: ['groups'],
    queryFn: async () => requireServerSuccess(await getGroups()),
    staleTime: 5 * 60 * 1000,
    enabled: props.open,
  })
  const groupOptions = useMemo(
    () => (groupsData?.data ?? []).map((g) => ({ label: g, value: g })),
    [groupsData]
  )

  useEffect(() => {
    if (!props.open) form.reset(COUPON_FORM_DEFAULT_VALUES)
  }, [props.open, form])

  const mutation = useMutation({
    mutationFn: async (values: CouponFormValues) => {
      const result = await createCoupon(transformCouponFormToPayload(values))
      if (!result.success) {
        throw createServerError(result, t(ERROR_MESSAGES.ISSUE_FAILED))
      }
      return result.data?.issued_count ?? 0
    },
    onSuccess: (count) => {
      toast.success(t('Issued coupons to {{count}} users', { count }))
      props.onOpenChange(false)
      void queryClient.invalidateQueries({ queryKey: couponsQueryKey })
    },
    onError: (error) => {
      handleServerError(error, t(ERROR_MESSAGES.ISSUE_FAILED))
    },
  })

  const isSubmitting = mutation.isPending

  return (
    <Sheet open={props.open} onOpenChange={props.onOpenChange}>
      <SheetContent className={sideDrawerContentClassName('sm:max-w-[600px]')}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>{t('Issue coupons')}</SheetTitle>
          <SheetDescription>
            {t('Coupons are issued to the selected users immediately.')}
          </SheetDescription>
        </SheetHeader>
        <Form {...form}>
          <form
            id='coupon-form'
            onSubmit={form.handleSubmit((values) => mutation.mutate(values))}
            className={sideDrawerFormClassName()}
          >
            <fieldset disabled={isSubmitting} className='contents'>
              <SideDrawerSection>
                <FormField
                  control={form.control}
                  name='name'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Name')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          maxLength={64}
                          placeholder={t('Enter a name')}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='scope'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Applies to')}</FormLabel>
                      <FormControl>
                        <RadioGroup
                          value={field.value}
                          onValueChange={(value) => {
                            if (
                              COUPON_SCOPE_OPTIONS.includes(
                                value as CouponScope
                              )
                            ) {
                              field.onChange(value)
                            }
                          }}
                          className='flex flex-wrap gap-x-5 gap-y-3'
                        >
                          {COUPON_SCOPE_OPTIONS.map((scope) => (
                            <div
                              key={scope}
                              className='flex items-center gap-2'
                            >
                              <RadioGroupItem
                                value={scope}
                                id={`coupon-scope-${scope}`}
                              />
                              <Label htmlFor={`coupon-scope-${scope}`}>
                                {t(COUPON_SCOPE_LABELS[scope])}
                              </Label>
                            </div>
                          ))}
                        </RadioGroup>
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <div className='grid gap-4 sm:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='threshold'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Minimum spend')}</FormLabel>
                        <FormControl>
                          <Input
                            type='number'
                            min='0'
                            step='0.01'
                            inputMode='decimal'
                            name={field.name}
                            ref={field.ref}
                            onBlur={field.onBlur}
                            value={Number.isNaN(field.value) ? '' : field.value}
                            onChange={(e) =>
                              field.onChange(parseAmountInput(e.target.value))
                            }
                          />
                        </FormControl>
                        <FormDescription>
                          {t('0 means no minimum')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='discount'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Discount amount')}</FormLabel>
                        <FormControl>
                          <Input
                            type='number'
                            min='0.01'
                            step='0.01'
                            inputMode='decimal'
                            name={field.name}
                            ref={field.ref}
                            onBlur={field.onBlur}
                            value={Number.isNaN(field.value) ? '' : field.value}
                            onChange={(e) =>
                              field.onChange(parseAmountInput(e.target.value))
                            }
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Uses the same currency as the payment amount')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                <FormField
                  control={form.control}
                  name='stackable'
                  render={({ field }) => (
                    <FormItem>
                      <label className='flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-sm'>
                        <span>{t('Allow stacking with other coupons')}</span>
                        <Switch
                          checked={field.value}
                          onCheckedChange={(checked) =>
                            field.onChange(!!checked)
                          }
                          aria-label={t('Allow stacking with other coupons')}
                        />
                      </label>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='never_expires'
                  render={({ field }) => (
                    <FormItem>
                      <label className='flex items-center justify-between gap-3 rounded-md border px-3 py-2 text-sm'>
                        <span>{t('Permanent')}</span>
                        <Switch
                          checked={field.value}
                          onCheckedChange={(checked) => {
                            field.onChange(!!checked)
                            if (checked) {
                              form.setValue('expires_at', null)
                              form.clearErrors('expires_at')
                            }
                          }}
                          aria-label={t('Permanent')}
                        />
                      </label>
                    </FormItem>
                  )}
                />

                {!neverExpires && (
                  <FormField
                    control={form.control}
                    name='expires_at'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Expiration Time')}</FormLabel>
                        <FormControl>
                          <DateTimePicker
                            value={field.value ?? undefined}
                            onChange={(date) => field.onChange(date ?? null)}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                )}

                <FormField
                  control={form.control}
                  name='target_type'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Recipients')}</FormLabel>
                      <FormControl>
                        <RadioGroup
                          value={field.value}
                          onValueChange={(value) => {
                            if (value === 'all' || value === 'custom') {
                              field.onChange(value)
                              form.clearErrors('user_ids_text')
                            }
                          }}
                          className='flex flex-wrap gap-x-5 gap-y-3'
                        >
                          {TARGET_OPTIONS.map((option) => (
                            <div
                              key={option.value}
                              className='flex items-center gap-2'
                            >
                              <RadioGroupItem
                                value={option.value}
                                id={`coupon-target-${option.value}`}
                              />
                              <Label htmlFor={`coupon-target-${option.value}`}>
                                {t(option.labelKey)}
                              </Label>
                            </div>
                          ))}
                        </RadioGroup>
                      </FormControl>
                      <FormDescription>
                        {targetType === 'all'
                          ? t('Issued to every enabled user that exists now.')
                          : t(
                              'Users matched by ID or group are combined; each user receives one coupon.'
                            )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {targetType === 'custom' && (
                  <>
                    <FormField
                      control={form.control}
                      name='user_ids_text'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('User IDs')}</FormLabel>
                          <FormControl>
                            <Textarea
                              {...field}
                              rows={3}
                              placeholder={t(
                                'Separate IDs with commas, spaces or new lines'
                              )}
                              className='font-mono'
                            />
                          </FormControl>
                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    <FormField
                      control={form.control}
                      name='groups'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel htmlFor='coupon-groups'>
                            {t('Groups')}
                          </FormLabel>
                          <MultiSelect
                            id='coupon-groups'
                            options={groupOptions}
                            selected={field.value}
                            onChange={(values) => {
                              field.onChange(values)
                              form.clearErrors('user_ids_text')
                            }}
                            placeholder={t('Select groups')}
                          />
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </>
                )}
              </SideDrawerSection>
            </fieldset>
          </form>
        </Form>
        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose render={<Button variant='outline' />}>
            {t('Close')}
          </SheetClose>
          <Button form='coupon-form' type='submit' disabled={isSubmitting}>
            {isSubmitting ? t('Issuing...') : t('Issue coupons')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
