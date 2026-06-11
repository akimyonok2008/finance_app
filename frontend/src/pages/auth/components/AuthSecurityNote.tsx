import { EyeOff, LockKeyhole, ShieldCheck } from "lucide-react";

const TRUST_ITEMS = [
  { icon: ShieldCheck, label: "Secure session" },
  { icon: EyeOff, label: "Private rankings" },
  { icon: LockKeyhole, label: "No public holdings" },
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
      <p className="text-xs leading-5 text-zinc-500">Private rankings. No public holdings.</p>

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
        <button
          type="button"
          className="underline underline-offset-2 hover:text-zinc-400 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400/40"
        >
          Privacy Policy
        </button>
        .
      </p>
    </div>
  );
}
