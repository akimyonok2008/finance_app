import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Copy, GitCompare } from "lucide-react";
import { toast } from "sonner";
import { useState } from "react";

import { compareProfile, copyFromProfile, copyPreview } from "@/api/strategy";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { queryKeys } from "@/hooks/queryKeys";
import type { CompareProfileResponse, CopyPreviewResponse } from "@/types/strategy";
import { cn } from "@/utils/cn";

type StrategyProfileActionsProps = {
  handle?: string;
  displayName?: string;
  canCopy?: boolean;
  compact?: boolean;
};

export function StrategyProfileActions({
  handle,
  displayName = "this strategy",
  canCopy = true,
  compact = false,
}: StrategyProfileActionsProps) {
  const queryClient = useQueryClient();
  const [copyOpen, setCopyOpen] = useState(false);
  const [compareOpen, setCompareOpen] = useState(false);

  const previewMutation = useMutation({
    mutationFn: () => copyPreview(handle ?? ""),
  });
  const copyMutation = useMutation({
    mutationFn: (preview: CopyPreviewResponse) =>
      copyFromProfile(handle ?? "", preview.weights),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: queryKeys.positions }),
        queryClient.invalidateQueries({ queryKey: queryKeys.portfolioSummary }),
        queryClient.invalidateQueries({ queryKey: queryKeys.leaderboard }),
        queryClient.invalidateQueries({ queryKey: queryKeys.leaderboardMe }),
        queryClient.invalidateQueries({ queryKey: queryKeys.myProfile }),
        queryClient.invalidateQueries({ queryKey: queryKeys.achievements }),
        queryClient.invalidateQueries({ queryKey: ["exploreProfiles"] }),
        handle
          ? queryClient.invalidateQueries({
              queryKey: queryKeys.publicProfile(handle),
            })
          : Promise.resolve(),
      ]);
      toast.success("Strategy baseline created from public weights.");
      setCopyOpen(false);
    },
    onError: (err) => {
      toast.error(err instanceof Error ? err.message : "Could not copy strategy.");
    },
  });
  const compareMutation = useMutation({
    mutationFn: () => compareProfile(handle ?? ""),
  });

  if (!handle) return null;

  const openCopy = () => {
    setCopyOpen(true);
    previewMutation.mutate();
  };
  const openCompare = () => {
    setCompareOpen(true);
    compareMutation.mutate();
  };

  return (
    <>
      <div className={cn("flex flex-wrap gap-2", compact && "justify-end")}>
        <Button type="button" variant="outline" size="sm" onClick={openCompare}>
          <GitCompare className="h-3.5 w-3.5" />
          Compare
        </Button>
        {canCopy && (
          <Button type="button" variant="outline" size="sm" onClick={openCopy}>
            <Copy className="h-3.5 w-3.5" />
            Copy weights
          </Button>
        )}
      </div>

      <CopyDialog
        open={copyOpen}
        onOpenChange={setCopyOpen}
        preview={previewMutation.data}
        isLoading={previewMutation.isPending}
        isError={previewMutation.isError}
        onRetry={() => previewMutation.mutate()}
        onConfirm={() => {
          if (previewMutation.data) copyMutation.mutate(previewMutation.data);
        }}
        isConfirming={copyMutation.isPending}
      />

      <CompareDialog
        open={compareOpen}
        onOpenChange={setCompareOpen}
        data={compareMutation.data}
        fallbackName={displayName}
        isLoading={compareMutation.isPending}
        isError={compareMutation.isError}
        onRetry={() => compareMutation.mutate()}
        onCopy={
          canCopy
            ? () => {
                setCompareOpen(false);
                openCopy();
              }
            : undefined
        }
      />
    </>
  );
}

function CopyDialog({
  open,
  onOpenChange,
  preview,
  isLoading,
  isError,
  onRetry,
  onConfirm,
  isConfirming,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  preview?: CopyPreviewResponse;
  isLoading: boolean;
  isError: boolean;
  onRetry: () => void;
  onConfirm: () => void;
  isConfirming: boolean;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl border-zinc-800 bg-zinc-950">
        <DialogHeader>
          <DialogTitle>Copy public strategy weights</DialogTitle>
          <DialogDescription>
            This creates your own strategy baseline at today&apos;s prices. No trades are executed.
          </DialogDescription>
        </DialogHeader>

        {isLoading ? (
          <p className="text-sm text-zinc-500">Loading public weights...</p>
        ) : isError ? (
          <div className="rounded-xl border border-rose-400/15 bg-rose-400/[0.04] p-4">
            <p className="text-sm text-rose-200">This strategy cannot be copied.</p>
            <Button variant="outline" size="sm" className="mt-3" onClick={onRetry}>
              Retry
            </Button>
          </div>
        ) : preview ? (
          <div className="space-y-4">
            <div className="rounded-xl border border-zinc-800 bg-zinc-900/40 p-4">
              <p className="text-sm font-medium text-zinc-100">
                {preview.source_profile.display_name}
              </p>
              <p className="font-mono text-xs text-zinc-500">
                @{preview.source_profile.handle}
              </p>
            </div>
            <div className="max-h-64 divide-y divide-zinc-800 overflow-auto rounded-xl border border-zinc-800">
              {preview.weights.map((weight) => (
                <div key={weight.symbol} className="flex items-center justify-between px-4 py-3">
                  <div>
                    <p className="font-mono text-sm text-zinc-100">{weight.symbol}</p>
                    <p className="text-xs capitalize text-zinc-500">{weight.asset_type}</p>
                  </div>
                  <p className="font-mono text-sm tabular-nums text-zinc-300">
                    {weight.weight_percentage.toFixed(1)}%
                  </p>
                </div>
              ))}
            </div>
            <p className="text-xs leading-5 text-zinc-500">
              Amounts, quantities, cost basis, and portfolio values are never copied.
            </p>
            <p className="text-xs text-zinc-600">{preview.disclaimer}</p>
          </div>
        ) : null}

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            type="button"
            disabled={!preview || isConfirming}
            onClick={onConfirm}
          >
            {isConfirming ? "Locking..." : "Lock my baseline"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function CompareDialog({
  open,
  onOpenChange,
  data,
  fallbackName,
  isLoading,
  isError,
  onRetry,
  onCopy,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  data?: CompareProfileResponse;
  fallbackName: string;
  isLoading: boolean;
  isError: boolean;
  onRetry: () => void;
  onCopy?: () => void;
}) {
  const name = data?.target_profile.display_name ?? fallbackName;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl border-zinc-800 bg-zinc-950">
        <DialogHeader>
          <DialogTitle>Compare with {name}</DialogTitle>
          <DialogDescription>
            Educational comparison using public strategy weights only.
          </DialogDescription>
        </DialogHeader>

        {isLoading ? (
          <p className="text-sm text-zinc-500">Comparing strategies...</p>
        ) : isError ? (
          <div className="rounded-xl border border-rose-400/15 bg-rose-400/[0.04] p-4">
            <p className="text-sm text-rose-200">
              Compare is unavailable. You may need your own strategy baseline first.
            </p>
            <Button variant="outline" size="sm" className="mt-3" onClick={onRetry}>
              Retry
            </Button>
          </div>
        ) : data ? (
          <div className="space-y-5">
            <div className="grid gap-3 sm:grid-cols-3">
              <Metric label="Overlap" value={`${data.overlap_score.toFixed(1)}%`} />
              <Metric
                label="Your top 3"
                value={`${data.concentration_comparison.my_top3_weight_percentage.toFixed(1)}%`}
              />
              <Metric
                label="Their top 3"
                value={`${data.concentration_comparison.target_top3_weight_percentage.toFixed(1)}%`}
              />
            </div>
            <p className="text-sm leading-6 text-zinc-300">{data.summary}</p>
            <div>
              <h3 className="text-xs font-semibold uppercase tracking-[0.14em] text-zinc-500">
                Weight gaps
              </h3>
              <div className="mt-2 max-h-56 divide-y divide-zinc-800 overflow-auto rounded-xl border border-zinc-800">
                {data.weight_differences.map((diff) => (
                  <div key={diff.symbol} className="grid gap-2 px-4 py-3 sm:grid-cols-[80px_1fr]">
                    <p className="font-mono text-sm text-zinc-100">{diff.symbol}</p>
                    <p className="font-mono text-xs tabular-nums text-zinc-400">
                      You {diff.my_weight_percentage.toFixed(1)}% vs {name}{" "}
                      {diff.target_weight_percentage.toFixed(1)}% {"->"}{" "}
                      <span
                        className={
                          diff.difference_percentage >= 0 ? "text-emerald-300" : "text-rose-300"
                        }
                      >
                        {diff.difference_percentage > 0 ? "+" : ""}
                        {diff.difference_percentage.toFixed(1)}%
                      </span>
                    </p>
                  </div>
                ))}
              </div>
            </div>
            {data.shared_symbols.length > 0 && (
              <div className="flex flex-wrap gap-1.5">
                {data.shared_symbols.map((symbol) => (
                  <span key={symbol} className="rounded-md border border-zinc-800 px-2 py-1 font-mono text-[10px] text-zinc-400">
                    {symbol}
                  </span>
                ))}
              </div>
            )}
            <div className="space-y-2">
              {data.learning_points.map((point) => (
                <div key={point.title} className="rounded-xl border border-zinc-800 bg-zinc-900/35 p-3">
                  <p className="text-xs font-medium text-zinc-200">{point.title}</p>
                  <p className="mt-1 text-xs leading-5 text-zinc-500">{point.body}</p>
                </div>
              ))}
            </div>
            <p className="text-xs text-zinc-600">{data.disclaimer}</p>
          </div>
        ) : null}

        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Close
          </Button>
          {onCopy && (
            <Button type="button" onClick={onCopy}>
              <Copy className="h-3.5 w-3.5" /> Copy weights
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl border border-zinc-800 bg-zinc-900/35 px-4 py-3">
      <p className="text-[10px] uppercase tracking-[0.14em] text-zinc-600">{label}</p>
      <p className="mt-2 font-mono text-lg font-semibold tabular-nums text-zinc-100">
        {value}
      </p>
    </div>
  );
}
