import { useState } from "react";
import { Flag } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useReportMessage, useReportUser } from "@/hooks/useSafety";
import { REPORT_CATEGORIES, type ReportCategory } from "@/types/safety";

const CATEGORY_LABELS: Record<ReportCategory, string> = {
  harassment: "Harassment",
  spam: "Spam",
  impersonation: "Impersonation",
  fraud_or_scam: "Fraud or scam",
  hate_or_abuse: "Hate or abuse",
  sexual_content: "Sexual content",
  threats: "Threats",
  privacy_violation: "Privacy violation",
  other: "Other",
};

type Props =
  | { target: "user"; userId: string; trigger?: React.ReactNode }
  | { target: "message"; messageId: string; trigger?: React.ReactNode };

/** Report modal for either a user account or a single DM message. The
 * reported party is never told a report was filed or who filed it. */
export function ReportModal(props: Props) {
  const [open, setOpen] = useState(false);
  const [category, setCategory] = useState<ReportCategory>("harassment");
  const [explanation, setExplanation] = useState("");
  const reportUser = useReportUser();
  const reportMessage = useReportMessage();
  const busy = reportUser.isPending || reportMessage.isPending;

  const submit = async () => {
    if (props.target === "user") {
      await reportUser.mutateAsync({ userId: props.userId, category, explanation });
    } else {
      await reportMessage.mutateAsync({ messageId: props.messageId, category, explanation });
    }
    setOpen(false);
    setExplanation("");
  };

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        {props.trigger ?? (
          <Button variant="outline" size="sm">
            <Flag className="h-3.5 w-3.5" /> Report
          </Button>
        )}
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{props.target === "user" ? "Report user" : "Report message"}</DialogTitle>
          <DialogDescription>
            Your report is confidential. The reported account will not be told who reported them.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          <div>
            <label className="mb-1 block text-xs font-medium text-zinc-400">Reason</label>
            <Select value={category} onValueChange={(v) => setCategory(v as ReportCategory)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {REPORT_CATEGORIES.map((c) => (
                  <SelectItem key={c} value={c}>
                    {CATEGORY_LABELS[c]}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-zinc-400">
              Additional details (optional)
            </label>
            <textarea
              className="min-h-24 w-full rounded-lg border border-white/10 bg-black/20 p-2 text-sm text-zinc-100 outline-none focus:border-indigo-400/40"
              value={explanation}
              onChange={(e) => setExplanation(e.target.value)}
              maxLength={2000}
              placeholder="What happened?"
            />
          </div>
        </div>

        <DialogFooter>
          <Button variant="ghost" onClick={() => setOpen(false)} disabled={busy}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={busy}>
            Submit report
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
