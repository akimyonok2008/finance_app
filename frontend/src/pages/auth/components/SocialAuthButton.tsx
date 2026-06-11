import { motion } from "framer-motion";
import { Loader2 } from "lucide-react";

type Props = {
  onClick: () => void;
  disabled?: boolean;
  loading?: boolean;
};

function GoogleMark() {
  return (
    <span
      className="mr-2 inline-flex h-5 w-5 items-center justify-center rounded-full bg-white text-xs font-bold leading-none text-zinc-800"
      aria-hidden
    >
      G
    </span>
  );
}

export function SocialAuthButton({ onClick, disabled, loading }: Props) {
  return (
    <motion.button
      type="button"
      whileHover={{ y: -1 }}
      whileTap={{ scale: 0.98 }}
      transition={{ type: "spring", stiffness: 400, damping: 20 }}
      onClick={onClick}
      disabled={disabled || loading}
      className="flex h-11 w-full items-center justify-center rounded-xl border border-zinc-800 bg-zinc-900/60 text-sm font-medium text-zinc-100 transition hover:border-zinc-700 hover:bg-zinc-900 disabled:cursor-not-allowed disabled:opacity-60"
      aria-label="Continue with Google"
    >
      {loading ? (
        <Loader2 className="h-4 w-4 animate-spin" />
      ) : (
        <>
          <GoogleMark />
          Continue with Google
        </>
      )}
    </motion.button>
  );
}
