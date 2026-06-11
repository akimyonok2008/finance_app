import { cva } from "class-variance-authority";

export const badgeVariants = cva(
  "inline-flex items-center rounded-md border px-2 py-0.5 text-[11px] font-medium uppercase tracking-wide transition-colors",
  {
    variants: {
      variant: {
        default: "border-white/10 bg-white/[0.04] text-slate-300",
        accent: "border-indigo-500/30 bg-indigo-500/10 text-indigo-300",
        emerald: "border-emerald-500/30 bg-emerald-500/10 text-emerald-300",
        crypto: "border-amber-500/30 bg-amber-500/10 text-amber-300",
      },
    },
    defaultVariants: {
      variant: "default",
    },
  },
);
