import { EyeOff, LockKeyhole, ShieldCheck } from "lucide-react";
import { Link } from "react-router-dom";

const TRUST_ITEMS = [
  { icon: ShieldCheck, label: "Secure session" },
  { icon: EyeOff, label: "Private account values" },
  { icon: LockKeyhole, label: "Opt-in composition" },
];

export function AuthSecurityNote() {
  return (
    <div className="mt-6 space-y-4 text-center">
      {/* Trust row */}
      <div className="flex items-center justify-center gap-4">
        {TRUST_ITEMS.map(({ icon: Icon, label }) => (
          <div
            key={label}
            className="flex items-center gap-1.5 text-[11px] text-zinc-500"
          >
            <Icon className="h-3 w-3 shrink-0 text-zinc-600" />
            <span>{label}</span>
          </div>
        ))}
      </div>

      {/* Main note */}
      <p className="text-xs leading-5 text-zinc-500">
        Rankings use percentages. You choose whether your public profile shares
        symbols and allocation weights.
      </p>

      {/* Legal */}
      <p className="text-xs text-zinc-600">
        By continuing, you agree to our{" "}
        <button
          type="button"
          className="underline underline-offset-2 hover:text-zinc-400 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400/40"
        >
          Terms
        </button>{" "}
        and{" "}
        <Link
          to="/privacy"
          className="underline underline-offset-2 hover:text-zinc-400 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400/40"
        >
          Privacy Policy
        </Link>
        .
      </p>
    </div>
  );
}
