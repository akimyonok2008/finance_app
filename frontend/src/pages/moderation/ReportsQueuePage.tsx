import { useState } from "react";

import { AppNav } from "@/components/layout/AppNav";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAuth } from "@/auth/useAuth";
import {
  useModerationReport,
  useModerationReports,
  useResolveReport,
} from "@/hooks/useSafety";
import type { ResolveReportInput } from "@/types/safety";

const ALL_STATUSES = "all";
const STATUS_FILTERS = [
  ALL_STATUSES,
  "open",
  "under_review",
  "resolved_action_taken",
  "resolved_no_action",
] as const;

/** Minimal moderator/admin report queue: list, evidence view, resolution
 * form. Not shown to ordinary users — gated on the current account's role. */
export function ReportsQueuePage() {
  const { user } = useAuth();
  const [status, setStatus] = useState<string>("open");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const reports = useModerationReports(status === ALL_STATUSES ? undefined : status);
  const detail = useModerationReport(selectedId);
  const resolve = useResolveReport();

  if (user && user.role !== "moderator" && user.role !== "admin") {
    return (
      <div className="min-h-screen bg-zinc-950 p-8 text-zinc-100">
        <p>You don't have access to this page.</p>
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50">
      <main className="mx-auto w-full max-w-6xl px-4 pb-16 pt-4 sm:px-6 lg:px-8">
        <AppNav />
        <h1 className="mb-4 text-2xl font-semibold">Moderation queue</h1>

        <div className="mb-4 max-w-xs">
          <Select value={status} onValueChange={setStatus}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {STATUS_FILTERS.map((s) => (
                <SelectItem key={s} value={s}>
                  {s}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>

        <div className="grid gap-5 lg:grid-cols-[1fr_1.2fr]">
          <div className="space-y-2">
            {reports.isLoading ? (
              <p className="text-sm text-zinc-500">Loading…</p>
            ) : reports.data?.reports.length === 0 ? (
              <p className="text-sm text-zinc-500">No reports.</p>
            ) : (
              reports.data?.reports.map((r) => (
                <button
                  key={r.id}
                  type="button"
                  onClick={() => setSelectedId(r.id)}
                  className={`block w-full rounded-lg border px-3 py-2 text-left text-sm ${
                    selectedId === r.id
                      ? "border-sky-400/40 bg-sky-400/10"
                      : "border-zinc-800 bg-zinc-900/40 hover:border-zinc-700"
                  }`}
                >
                  <div className="font-medium">{r.category}</div>
                  <div className="text-xs text-zinc-500">
                    {r.status} · {new Date(r.created_at).toLocaleString()}
                    {r.message_id ? " · message" : " · user"}
                  </div>
                </button>
              ))
            )}
          </div>

          <div className="rounded-xl border border-zinc-800 bg-zinc-900/40 p-4">
            {!selectedId ? (
              <p className="text-sm text-zinc-500">Select a report to review.</p>
            ) : detail.isLoading ? (
              <p className="text-sm text-zinc-500">Loading…</p>
            ) : detail.data ? (
              <ReportDetailPanel
                report={detail.data}
                onResolve={(input) => resolve.mutate({ reportId: detail.data.id, input })}
                resolving={resolve.isPending}
              />
            ) : null}
          </div>
        </div>
      </main>
    </div>
  );
}

function ReportDetailPanel({
  report,
  onResolve,
  resolving,
}: {
  report: NonNullable<ReturnType<typeof useModerationReport>["data"]>;
  onResolve: (input: ResolveReportInput) => void;
  resolving: boolean;
}) {
  const [actionType, setActionType] = useState<NonNullable<ResolveReportInput["action_type"]>>(
    "warning",
  );
  const [reason, setReason] = useState("");
  const [notes, setNotes] = useState("");
  const [days, setDays] = useState(3);
  const resolved = report.status.startsWith("resolved");

  return (
    <div className="space-y-4">
      <div>
        <div className="text-sm text-zinc-400">Category</div>
        <div className="font-medium">{report.category}</div>
      </div>
      {report.explanation && (
        <div>
          <div className="text-sm text-zinc-400">Reporter's explanation</div>
          <p className="text-sm">{report.explanation}</p>
        </div>
      )}
      {report.evidence && (
        <div className="rounded-lg border border-amber-400/20 bg-amber-400/[0.04] p-3">
          <div className="mb-1 text-xs uppercase tracking-wide text-amber-300/70">
            Message evidence
          </div>
          <p className="whitespace-pre-wrap text-sm">{report.evidence.message_text}</p>
        </div>
      )}

      {resolved ? (
        <div className="rounded-lg border border-zinc-800 bg-zinc-950/40 p-3 text-sm">
          <div>Decision: {report.decision}</div>
          {report.moderator_notes && <div>Notes: {report.moderator_notes}</div>}
        </div>
      ) : (
        <div className="space-y-3 border-t border-zinc-800 pt-4">
          <div>
            <label className="mb-1 block text-xs text-zinc-400">Action</label>
            <Select value={actionType} onValueChange={(v) => setActionType(v as typeof actionType)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="warning">Warning</SelectItem>
                <SelectItem value="temporary_suspension">Temporary suspension</SelectItem>
                <SelectItem value="permanent_ban">Permanent ban</SelectItem>
                <SelectItem value="content_removal">Content removal</SelectItem>
              </SelectContent>
            </Select>
          </div>
          {actionType === "temporary_suspension" && (
            <div>
              <label className="mb-1 block text-xs text-zinc-400">Suspension days</label>
              <input
                type="number"
                min={1}
                value={days}
                onChange={(e) => setDays(Number(e.target.value))}
                className="w-24 rounded-lg border border-white/10 bg-black/20 p-1.5 text-sm"
              />
            </div>
          )}
          <div>
            <label className="mb-1 block text-xs text-zinc-400">Reason (shown in audit log)</label>
            <input
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              className="w-full rounded-lg border border-white/10 bg-black/20 p-1.5 text-sm"
            />
          </div>
          <div>
            <label className="mb-1 block text-xs text-zinc-400">Moderator notes (internal)</label>
            <textarea
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              className="min-h-16 w-full rounded-lg border border-white/10 bg-black/20 p-1.5 text-sm"
            />
          </div>
          <div className="flex gap-2">
            <Button
              disabled={resolving}
              onClick={() =>
                onResolve({
                  decision: "action_taken",
                  action_type: actionType,
                  reason,
                  moderator_notes: notes,
                  suspension_days: days,
                })
              }
            >
              Apply action
            </Button>
            <Button
              variant="outline"
              disabled={resolving}
              onClick={() => onResolve({ decision: "no_action", moderator_notes: notes })}
            >
              Dismiss (no action)
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
