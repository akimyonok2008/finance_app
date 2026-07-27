import { LoaderCircle } from "lucide-react";

import {
  usePortfolioSettings,
  useUpdatePortfolioSettings,
} from "@/hooks/usePortfolioSettings";
import { cn } from "@/utils/cn";

function Toggle({
  checked,
  onChange,
  disabled,
  label,
  description,
}: {
  checked: boolean;
  onChange: (value: boolean) => void;
  disabled?: boolean;
  label: string;
  description: string;
}) {
  return (
    <div className="flex items-start justify-between gap-4 rounded-xl border border-emerald-300/10 bg-emerald-400/[0.035] p-4">
      <div>
        <div className="text-sm font-medium text-zinc-200">{label}</div>
        <p className="mt-1 text-xs leading-5 text-zinc-500">{description}</p>
      </div>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        aria-label={label}
        disabled={disabled}
        onClick={() => onChange(!checked)}
        className={cn(
          "relative mt-0.5 h-6 w-11 shrink-0 rounded-full border transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500 disabled:cursor-not-allowed disabled:opacity-60",
          checked ? "border-emerald-300/60 bg-emerald-300" : "border-zinc-700 bg-zinc-900",
        )}
      >
        <span
          className={cn(
            "absolute top-0.5 h-4 w-4 rounded-full transition",
            checked ? "left-5 bg-emerald-950" : "left-1 bg-zinc-500",
          )}
        />
      </button>
    </div>
  );
}

export function PortfolioSettingsCard() {
  const query = usePortfolioSettings();
  const mutation = useUpdatePortfolioSettings();

  return (
    <div className="rounded-2xl border border-emerald-300/15 bg-gradient-to-br from-emerald-400/[0.055] to-zinc-900/70 p-5 shadow-lg shadow-emerald-950/10 sm:p-6">
      <h2 className="text-base font-semibold text-zinc-50">Purchase funding</h2>
      <p className="mt-1 text-xs text-zinc-400">
        Behavior for how new buys are funded.
      </p>

      <div className="mt-5">
        {query.isLoading ? (
          <div className="flex items-center gap-2 text-xs text-zinc-500">
            <LoaderCircle className="h-3.5 w-3.5 animate-spin" />
            Loading…
          </div>
        ) : query.isError || !query.data ? (
          <p className="text-xs text-rose-300">
            Could not load portfolio settings.
          </p>
        ) : (
          <Toggle
            checked={query.data.auto_fund_purchases}
            disabled={mutation.isPending}
            onChange={(value) =>
              mutation.mutate({ auto_fund_purchases: value })
            }
            label="Automatically fund purchases"
            description="When on (default), a buy that costs more than your available cash automatically records the shortfall as a deposit. When off, that buy is rejected instead — buys always require sufficient cash."
          />
        )}
      </div>
    </div>
  );
}
