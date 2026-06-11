import { AnimatePresence, motion } from "framer-motion";
import { type UseFormRegisterReturn } from "react-hook-form";

type Props = {
  id: string;
  label: string;
  type?: string;
  registration: UseFormRegisterReturn;
  error?: string;
  disabled?: boolean;
  autoComplete?: string;
};

export function FloatingLabelInput({
  id,
  label,
  type = "text",
  registration,
  error,
  disabled,
  autoComplete,
}: Props) {
  const errorId = `${id}-error`;

  return (
    <div className="relative">
      <input
        id={id}
        type={type}
        placeholder=" "
        disabled={disabled}
        autoComplete={autoComplete}
        aria-invalid={!!error}
        aria-describedby={error ? errorId : undefined}
        className="peer h-12 w-full rounded-xl border border-zinc-800 bg-zinc-950/70 px-4 pb-1 pt-5 text-sm text-zinc-50 outline-none transition placeholder:text-transparent focus:border-violet-400/70 focus:ring-2 focus:ring-violet-400/20 disabled:cursor-not-allowed disabled:opacity-60 aria-[invalid=true]:border-rose-400/60 aria-[invalid=true]:focus:border-rose-400/60 aria-[invalid=true]:focus:ring-rose-400/20"
        {...registration}
      />
      <label
        htmlFor={id}
        className="pointer-events-none absolute left-4 top-3 text-xs text-zinc-500 transition-all peer-placeholder-shown:top-3.5 peer-placeholder-shown:text-sm peer-focus:top-1.5 peer-focus:text-[11px] peer-focus:text-violet-300 peer-[:not(:placeholder-shown)]:top-1.5 peer-[:not(:placeholder-shown)]:text-[11px]"
      >
        {label}
      </label>

      <AnimatePresence>
        {error && (
          <motion.p
            id={errorId}
            role="alert"
            initial={{ opacity: 0, y: -4 }}
            animate={{ opacity: 1, y: 0, x: [0, -4, 4, -2, 2, 0] }}
            exit={{ opacity: 0, y: -4 }}
            transition={{ duration: 0.28 }}
            className="mt-1.5 text-xs text-rose-300"
          >
            {error}
          </motion.p>
        )}
      </AnimatePresence>
    </div>
  );
}
