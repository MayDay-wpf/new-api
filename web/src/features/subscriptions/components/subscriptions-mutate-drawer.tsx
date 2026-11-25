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
import { zodResolver } from "@hookform/resolvers/zod";
import {
  CalendarClock,
  CreditCard,
  Plus,
  RefreshCw,
  Settings2,
  Trash2,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useFieldArray, useForm, type Resolver } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

import {
  SideDrawerSection,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
  sideDrawerSwitchItemClassName,
} from "@/components/drawer-layout";
import { Button } from "@/components/ui/button";
import { Combobox } from "@/components/ui/combobox";
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { IconBadge } from "@/components/ui/icon-badge";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Switch } from "@/components/ui/switch";
import { getPaymentErrorTranslation } from "@/features/wallet/lib/payment";
import { getCurrencyDisplay, getCurrencyLabel } from "@/lib/currency";
import { handleServerError } from "@/lib/handle-server-error";

import {
  createPlan,
  updatePlan,
  getGroups,
  createWaffoPancakeSubscriptionProduct,
  listWaffoPancakeSubscriptionProductOptions,
} from "../api";
import {
  getDurationUnitOptions,
  getQuotaLimitPeriodOptions,
} from "../constants";
import {
  getPlanCurrencySymbol,
  getPlanFormSchema,
  PLAN_CURRENCIES,
  PLAN_FORM_DEFAULTS,
  planToFormValues,
  formValuesToPlanPayload,
  normalizePlanCurrency,
  type PlanFormValues,
} from "../lib";
import type { PlanRecord } from "../types";
import { useSubscriptions } from "./subscriptions-provider";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  currentRow?: PlanRecord;
}

export function SubscriptionsMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: Props) {
  const { t } = useTranslation();
  const isEdit = !!currentRow?.plan?.id;
  const { triggerRefresh } = useSubscriptions();
  const { meta: currencyMeta } = getCurrencyDisplay();
  const tokensOnly = currencyMeta.kind === "tokens";
  const currencyLabel = getCurrencyLabel();
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [groupOptions, setGroupOptions] = useState<string[]>([]);
  const [creatingPancakeProduct, setCreatingPancakeProduct] = useState(false);
  const [pancakeProducts, setPancakeProducts] = useState<
    { id: string; name: string; status: string }[]
  >([]);

  const schema = getPlanFormSchema(t);
  const form = useForm<PlanFormValues>({
    resolver: zodResolver(schema) as unknown as Resolver<PlanFormValues>,
    defaultValues: PLAN_FORM_DEFAULTS,
  });
  const {
    fields: quotaLimitFields,
    append: appendQuotaLimit,
    remove: removeQuotaLimit,
  } = useFieldArray({
    control: form.control,
    name: "quota_limits",
  });

  useEffect(() => {
    if (open) {
      if (currentRow?.plan) {
        form.reset(planToFormValues(currentRow.plan));
      } else {
        form.reset(PLAN_FORM_DEFAULTS);
      }
      getGroups()
        .then((res) => {
          if (res.success) {
            setGroupOptions(res.data || []);
          } else {
            handleServerError(res);
          }
        })
        .catch(() => {});
      // Best-effort — empty list still lets the operator use "+ Create".
      listWaffoPancakeSubscriptionProductOptions()
        .then((res) => {
          if (
            res.message === "success" &&
            typeof res.data === "object" &&
            res.data &&
            Array.isArray((res.data as { products?: unknown }).products)
          ) {
            setPancakeProducts(
              (res.data as { products: typeof pancakeProducts }).products,
            );
          } else {
            setPancakeProducts([]);
          }
        })
        .catch(() => setPancakeProducts([]));
    }
  }, [open, currentRow, form]);

  const durationUnit = form.watch("duration_unit");
  const quotaLimits = form.watch("quota_limits");
  // Gate "+ Create on Pancake" on the same checks the mint handler runs.
  const watchedTitle = form.watch("title");
  const watchedPrice = form.watch("price_amount");
  const watchedCurrency = form.watch("currency");
  // PLAN_CURRENCIES is the curated list, but a plan saved earlier with another
  // ISO code must keep its value selectable instead of silently rendering empty.
  const currencyOptions = useMemo(() => {
    const codes = new Set<string>(PLAN_CURRENCIES);
    const storedCurrency = currentRow?.plan?.currency;
    if (storedCurrency) {
      codes.add(normalizePlanCurrency(storedCurrency));
    }
    if (watchedCurrency) {
      codes.add(normalizePlanCurrency(watchedCurrency));
    }
    return [...codes].map((code) => ({
      value: code,
      label: `${code} · ${getPlanCurrencySymbol(code)}`,
    }));
  }, [currentRow?.plan?.currency, watchedCurrency]);
  const pancakeCreateReady =
    typeof watchedTitle === "string" &&
    watchedTitle.trim().length > 0 &&
    Number(watchedPrice ?? 0) > 0;

  const onSubmit = async (values: PlanFormValues) => {
    setIsSubmitting(true);
    try {
      const payload = formValuesToPlanPayload(values);
      if (isEdit && currentRow?.plan?.id) {
        const res = await updatePlan(currentRow.plan.id, payload);
        if (res.success) {
          toast.success(t("Update succeeded"));
          onOpenChange(false);
          triggerRefresh();
        } else {
          handleServerError(res);
        }
      } else {
        const res = await createPlan(payload);
        if (res.success) {
          toast.success(t("Create succeeded"));
          onOpenChange(false);
          triggerRefresh();
        } else {
          handleServerError(res);
        }
      }
    } catch (error) {
      handleServerError(error, t("Request failed"));
    } finally {
      setIsSubmitting(false);
    }
  };

  // Mints a Pancake OnetimeProduct (not SubscriptionProduct — see
  // controller) using persisted creds + the form's title/price, then
  // pins the returned PROD_ ID into the form field.
  const handleCreatePancakeProduct = async () => {
    const title = form.getValues("title").trim();
    const priceAmount = Number(form.getValues("price_amount") || 0);
    if (!title) {
      toast.error(t("Plan title is required"));
      return;
    }
    if (priceAmount <= 0) {
      toast.error(t("Plan price must be greater than zero"));
      return;
    }
    setCreatingPancakeProduct(true);
    try {
      const res = await createWaffoPancakeSubscriptionProduct({
        name: title,
        amount: priceAmount.toFixed(2),
      });
      if (
        res.message === "success" &&
        typeof res.data === "object" &&
        res.data
      ) {
        const created = res.data as {
          product_id: string;
          product_name: string;
        };
        form.setValue("waffo_pancake_product_id", created.product_id, {
          shouldDirty: true,
        });
        // Refetch from GraphQL so the dropdown reflects authoritative state.
        try {
          const refresh = await listWaffoPancakeSubscriptionProductOptions();
          if (
            refresh.message === "success" &&
            typeof refresh.data === "object" &&
            refresh.data &&
            Array.isArray((refresh.data as { products?: unknown }).products)
          ) {
            setPancakeProducts(
              (refresh.data as { products: typeof pancakeProducts }).products,
            );
          }
        } catch {
          // Best-effort — form value already points at the new product;
          // raw-ID fallback covers the missing label.
        }
        toast.success(
          `${t("Waffo Pancake product created")}: ${created.product_id}`,
        );
      } else {
        const reason =
          getPaymentErrorTranslation(res) ??
          (typeof res.data === "string" ? res.data : undefined);
        handleServerError(res.data, undefined, {
          title: reason
            ? `${t("Waffo Pancake product creation failed")}: ${reason}`
            : t("Waffo Pancake product creation failed"),
        });
      }
    } catch (err) {
      handleServerError(
        err,
        `${t("Waffo Pancake product creation failed")}: ${err instanceof Error ? err.message : String(err)}`,
      );
    } finally {
      setCreatingPancakeProduct(false);
    }
  };

  const durationUnitOpts = getDurationUnitOptions(t);
  const quotaLimitPeriodOpts = [
    { value: "five_hours", label: t("5 Hours") },
    ...getQuotaLimitPeriodOptions(t),
  ];

  return (
    <Sheet
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v);
        if (!v) {
          form.reset();
        }
      }}
    >
      <SheetContent className={sideDrawerContentClassName("sm:max-w-[600px]")}>
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>
            {isEdit ? t("Update plan info") : t("Create new subscription plan")}
          </SheetTitle>
          <SheetDescription>
            {isEdit
              ? t("Modify existing subscription plan configuration")
              : t(
                  "Fill in the following info to create a new subscription plan",
                )}
          </SheetDescription>
        </SheetHeader>
        <Form {...form}>
          <form
            id="subscription-form"
            onSubmit={form.handleSubmit(onSubmit)}
            className={sideDrawerFormClassName()}
          >
            {/* Basic Info */}
            <SideDrawerSection>
              <h3 className="flex items-center gap-2 text-sm font-medium">
                <IconBadge tone="info" size="xs">
                  <Settings2 />
                </IconBadge>
                {t("Basic Info")}
              </h3>

              <FormField
                control={form.control}
                name="title"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("Plan Title")}</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder={t("e.g. Basic Plan")} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="subtitle"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("Plan Subtitle")}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        placeholder={t("e.g. Suitable for light usage")}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <FormField
                  control={form.control}
                  name="price_amount"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t("Plan Price")}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type="number"
                          step="0.01"
                          min={0}
                          onChange={(e) =>
                            field.onChange(
                              Number.parseFloat(e.target.value) || 0,
                            )
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          "Amount the user pays to purchase this plan; the actual currency depends on the payment gateway.",
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="currency"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t("Currency")}</FormLabel>
                      <Select
                        items={currencyOptions}
                        onValueChange={field.onChange}
                        value={field.value}
                      >
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent alignItemWithTrigger={false}>
                          <SelectGroup>
                            {currencyOptions.map((o) => (
                              <SelectItem key={o.value} value={o.value}>
                                {o.label}
                              </SelectItem>
                            ))}
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                      <FormDescription>
                        {t(
                          "Currency used to display this plan's price. The payment gateway still decides the currency actually charged.",
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="reset_price_multiplier"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t("Reset Price Markup")}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type="number"
                          step="0.01"
                          min={0}
                          onChange={(e) =>
                            field.onChange(
                              Number.parseFloat(e.target.value) || 0,
                            )
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          "The paid reset price is the plan price prorated to the longest quota window (a weekly window on a monthly plan costs about a quarter), then multiplied by (1 + this markup). 0.5 means 1.5 times the prorated price.",
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <div className="sm:col-span-2">
                  <FormField
                    control={form.control}
                    name="total_amount"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>
                          {t("Quota ({{currency}})", {
                            currency: currencyLabel,
                          })}
                        </FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            type="number"
                            min={0}
                            step={tokensOnly ? 1 : 0.01}
                            placeholder={
                              tokensOnly
                                ? t("Enter quota in tokens")
                                : t("Enter quota in {{currency}}", {
                                    currency: currencyLabel,
                                  })
                            }
                            onChange={(e) =>
                              field.onChange(
                                Number.parseFloat(e.target.value) || 0,
                              )
                            }
                          />
                        </FormControl>
                        <FormDescription>
                          {t(
                            "Total quota included in the plan. 0 means unlimited. Per-window caps are configured below.",
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
              </div>

              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <FormField
                  control={form.control}
                  name="upgrade_group"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t("Upgrade Group")}</FormLabel>
                      <FormControl>
                        <Combobox
                          options={[
                            { value: "__none__", label: t("No Upgrade") },
                            ...groupOptions.map((g) => ({
                              value: g,
                              label: g,
                            })),
                          ]}
                          onValueChange={(v) =>
                            field.onChange(v === "__none__" ? "" : v)
                          }
                          value={field.value || ""}
                          className="w-full"
                          placeholder={t("No Upgrade")}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="downgrade_group"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t("Downgrade Group")}</FormLabel>
                      <FormControl>
                        <Combobox
                          options={[
                            {
                              value: "__none__",
                              label: t("Downgrade to pre-purchase group"),
                            },
                            ...groupOptions.map((g) => ({
                              value: g,
                              label: g,
                            })),
                          ]}
                          onValueChange={(v) =>
                            field.onChange(v === "__none__" ? "" : v)
                          }
                          value={field.value || ""}
                          className="w-full"
                          placeholder={t("Downgrade to pre-purchase group")}
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          "Downgrade to this group after the subscription expires",
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="max_purchase_per_user"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t("Purchase Limit")}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type="number"
                          min={0}
                          onChange={(e) =>
                            field.onChange(
                              Number.parseInt(e.target.value, 10) || 0,
                            )
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          "Maximum active subscriptions of this plan per user. 0 means unlimited.",
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="stock_total"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t("Total Stock")}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type="number"
                          min={0}
                          onChange={(e) =>
                            field.onChange(
                              Number.parseInt(e.target.value, 10) || 0,
                            )
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {isEdit
                          ? t(
                              "Total units for sale across all users, 0 means unlimited. Sold: {{sold}}",
                              {
                                sold: Number(currentRow?.plan?.stock_sold || 0),
                              },
                            )
                          : t(
                              "Total units for sale across all users, 0 means unlimited",
                            )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <FormField
                control={form.control}
                name="sort_order"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t("Sort Order")}</FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type="number"
                        onChange={(e) =>
                          field.onChange(
                            Number.parseInt(e.target.value, 10) || 0,
                          )
                        }
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <div className="flex flex-col gap-3">
                <FormField
                  control={form.control}
                  name="enabled"
                  render={({ field }) => (
                    <FormItem className={sideDrawerSwitchItemClassName()}>
                      <FormLabel className="!mt-0">
                        {t("Enabled Status")}
                      </FormLabel>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="allow_balance_pay"
                  render={({ field }) => (
                    <FormItem className={sideDrawerSwitchItemClassName()}>
                      <FormLabel className="!mt-0">
                        {t("Allow balance redemption")}
                      </FormLabel>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="allow_wallet_overflow"
                  render={({ field }) => (
                    <FormItem className={sideDrawerSwitchItemClassName()}>
                      <FormLabel className="!mt-0">
                        {t("Allow wallet balance after quota used up")}
                      </FormLabel>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
              </div>
            </SideDrawerSection>

            {/* Duration Settings */}
            <SideDrawerSection>
              <h3 className="flex items-center gap-2 text-sm font-medium">
                <IconBadge tone="chart-4" size="xs">
                  <CalendarClock />
                </IconBadge>
                {t("Duration Settings")}
              </h3>

              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <FormField
                  control={form.control}
                  name="duration_unit"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t("Duration Unit")}</FormLabel>
                      <Select
                        items={durationUnitOpts.map((o) => ({
                          value: o.value,
                          label: o.label,
                        }))}
                        onValueChange={field.onChange}
                        value={field.value}
                      >
                        <FormControl>
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                        </FormControl>
                        <SelectContent alignItemWithTrigger={false}>
                          <SelectGroup>
                            {durationUnitOpts.map((o) => (
                              <SelectItem key={o.value} value={o.value}>
                                {o.label}
                              </SelectItem>
                            ))}
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                {durationUnit === "custom" ? (
                  <FormField
                    control={form.control}
                    name="custom_seconds"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t("Custom Seconds")}</FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            type="number"
                            min={1}
                            onChange={(e) =>
                              field.onChange(
                                Number.parseInt(e.target.value, 10) || 0,
                              )
                            }
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                ) : (
                  <FormField
                    control={form.control}
                    name="duration_value"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t("Duration Value")}</FormLabel>
                        <FormControl>
                          <Input
                            {...field}
                            type="number"
                            min={1}
                            onChange={(e) =>
                              field.onChange(
                                Number.parseInt(e.target.value, 10) || 0,
                              )
                            }
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                )}
              </div>
            </SideDrawerSection>

            {/* Quota Limits */}
            <SideDrawerSection>
              <h3 className="flex items-center gap-2 text-sm font-medium">
                <IconBadge tone="success" size="xs">
                  <RefreshCw />
                </IconBadge>
                {t("Quota Limits")}
              </h3>
              <FormDescription>
                {t(
                  "Multiple limits are enforced at the same time. Each window resets independently, while the total quota remains the lifetime cap.",
                )}
              </FormDescription>

              <div className="space-y-3">
                {quotaLimitFields.length === 0 && (
                  <p className="text-muted-foreground text-xs">
                    {t("No quota limits configured")}
                  </p>
                )}

                {quotaLimitFields.map((limitField, index) => {
                  const currentPeriod = quotaLimits?.[index]?.period;
                  const currentCustomSeconds = Number(
                    quotaLimits?.[index]?.custom_seconds || 0,
                  );
                  const windowValue =
                    currentPeriod === "custom" && currentCustomSeconds === 18000
                      ? "five_hours"
                      : (currentPeriod ?? "daily");

                  return (
                    <div
                      key={limitField.id}
                      className="space-y-3 rounded-lg border p-3"
                    >
                      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                        <FormField
                          control={form.control}
                          name={`quota_limits.${index}.period`}
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>{t("Limit Window")}</FormLabel>
                              <Select
                                items={quotaLimitPeriodOpts}
                                value={windowValue}
                                onValueChange={(value) => {
                                  if (value === "five_hours") {
                                    field.onChange("custom");
                                    form.setValue(
                                      `quota_limits.${index}.custom_seconds`,
                                      18000,
                                      {
                                        shouldDirty: true,
                                        shouldValidate: true,
                                      },
                                    );
                                    return;
                                  }
                                  field.onChange(value);
                                  if (value !== "custom") {
                                    form.setValue(
                                      `quota_limits.${index}.custom_seconds`,
                                      0,
                                      { shouldDirty: true },
                                    );
                                  }
                                }}
                              >
                                <FormControl>
                                  <SelectTrigger>
                                    <SelectValue />
                                  </SelectTrigger>
                                </FormControl>
                                <SelectContent alignItemWithTrigger={false}>
                                  <SelectGroup>
                                    {quotaLimitPeriodOpts.map((option) => (
                                      <SelectItem
                                        key={option.value}
                                        value={option.value}
                                      >
                                        {option.label}
                                      </SelectItem>
                                    ))}
                                  </SelectGroup>
                                </SelectContent>
                              </Select>
                              <FormMessage />
                            </FormItem>
                          )}
                        />

                        <FormField
                          control={form.control}
                          name={`quota_limits.${index}.amount`}
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>
                                {t("Limit Quota ({{currency}})", {
                                  currency: currencyLabel,
                                })}
                              </FormLabel>
                              <FormControl>
                                <Input
                                  {...field}
                                  type="number"
                                  min={0}
                                  step={tokensOnly ? 1 : 0.01}
                                  placeholder={
                                    tokensOnly
                                      ? t("Enter quota in tokens")
                                      : t("Enter quota in {{currency}}", {
                                          currency: currencyLabel,
                                        })
                                  }
                                  onChange={(event) =>
                                    field.onChange(
                                      Number.parseFloat(event.target.value) ||
                                        0,
                                    )
                                  }
                                />
                              </FormControl>
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                      </div>

                      {currentPeriod === "custom" && (
                        <FormField
                          control={form.control}
                          name={`quota_limits.${index}.custom_seconds`}
                          render={({ field }) => (
                            <FormItem>
                              <FormLabel>
                                {t("Custom Window (seconds)")}
                              </FormLabel>
                              <FormControl>
                                <Input
                                  {...field}
                                  type="number"
                                  min={1}
                                  onChange={(event) =>
                                    field.onChange(
                                      Number.parseInt(event.target.value, 10) ||
                                        0,
                                    )
                                  }
                                />
                              </FormControl>
                              <FormDescription>
                                {t(
                                  "18000 seconds = 5 hours. Custom windows start from the subscription start time.",
                                )}
                              </FormDescription>
                              <FormMessage />
                            </FormItem>
                          )}
                        />
                      )}

                      <div className="flex justify-end">
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="text-destructive hover:text-destructive"
                          onClick={() => removeQuotaLimit(index)}
                        >
                          <Trash2 className="mr-1 h-3.5 w-3.5" />
                          {t("Remove")}
                        </Button>
                      </div>
                    </div>
                  );
                })}

                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() =>
                    appendQuotaLimit({
                      period: "daily",
                      custom_seconds: 0,
                      amount: 0,
                    })
                  }
                >
                  <Plus className="mr-1 h-4 w-4" />
                  {t("Add Limit")}
                </Button>
              </div>
            </SideDrawerSection>

            {/* Payment Config */}
            <SideDrawerSection>
              <h3 className="flex items-center gap-2 text-sm font-medium">
                <IconBadge tone="warning" size="xs">
                  <CreditCard />
                </IconBadge>
                {t("Third-party Payment Config")}
              </h3>

              <FormField
                control={form.control}
                name="stripe_price_id"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Stripe Price ID</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder="price_..." />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="creem_product_id"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Creem Product ID</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder="prod_..." />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name="waffo_pancake_product_id"
                render={({ field }) => {
                  // Raw-ID fallback for IDs not yet in the catalog.
                  const items = pancakeProducts.map((p) => ({
                    value: p.id,
                    label: `${p.name} (${p.id})`,
                  }));
                  if (
                    field.value &&
                    !pancakeProducts.some((p) => p.id === field.value)
                  ) {
                    items.push({ value: field.value, label: field.value });
                  }
                  return (
                    <FormItem>
                      <FormLabel>Waffo Pancake Product ID</FormLabel>
                      <div className="flex gap-2">
                        <Combobox
                          options={items}
                          value={field.value || ""}
                          onValueChange={(v) => field.onChange(v)}
                          disabled={items.length === 0}
                          className="w-full flex-1"
                          placeholder={t("Select a product")}
                        />
                        <Button
                          type="button"
                          variant="outline"
                          onClick={handleCreatePancakeProduct}
                          disabled={
                            creatingPancakeProduct || !pancakeCreateReady
                          }
                          className="shrink-0"
                        >
                          {creatingPancakeProduct
                            ? t("Creating...")
                            : `+ ${t("Create")}`}
                        </Button>
                      </div>
                      <FormDescription>
                        {t(
                          "Creates a Pancake product in the saved store using this plan’s title and price. Requires Waffo Pancake to be fully configured in Payment settings first.",
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  );
                }}
              />
            </SideDrawerSection>
          </form>
        </Form>
        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose render={<Button variant="outline" />}>
            {t("Close")}
          </SheetClose>
          <Button
            form="subscription-form"
            type="submit"
            disabled={isSubmitting}
          >
            {isSubmitting ? t("Saving...") : t("Save changes")}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}
