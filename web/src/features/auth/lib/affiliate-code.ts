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
import i18next from "i18next";
import { toast } from "sonner";

import { getServerErrorSources } from "@/lib/server-error-message";

import { clearAffiliateCode } from "./storage";

export const AFFILIATE_CODE_UNAVAILABLE = "AFFILIATE_CODE_UNAVAILABLE";

/** The registration carried the invitation code of a disabled user. */
export function isAffiliateCodeUnavailable(value: unknown): boolean {
  return getServerErrorSources(value).some(
    (source) => source.code === AFFILIATE_CODE_UNAVAILABLE,
  );
}

/**
 * Tell the user the invitation link can no longer be used and offer to remove
 * the saved invitation code so they can register without it.
 */
export function promptRemoveAffiliateCode(): void {
  toast.error(
    i18next.t(
      "This invitation link is no longer valid. Remove the invitation code and register again.",
    ),
    {
      id: AFFILIATE_CODE_UNAVAILABLE,
      duration: Infinity,
      action: {
        label: i18next.t("Remove invitation code"),
        onClick: () => {
          clearAffiliateCode();
          toast.success(
            i18next.t("Invitation code removed. You can register now."),
          );
        },
      },
    },
  );
}
