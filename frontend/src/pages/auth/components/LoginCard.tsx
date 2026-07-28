import { AnimatePresence, motion } from "framer-motion";
import { ArrowLeft, ShieldCheck } from "lucide-react";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Link, useNavigate } from "react-router-dom";

import { AuthApiError } from "@/api/authApi";
import { useAuth } from "@/auth/useAuth";
import { AuthDivider } from "@/pages/auth/components/AuthDivider";
import { AuthLoadingSpinner } from "@/pages/auth/components/AuthLoadingSpinner";
import { AuthSecurityNote } from "@/pages/auth/components/AuthSecurityNote";
import { FloatingLabelInput } from "@/pages/auth/components/FloatingLabelInput";
import { OAuthButtons } from "@/pages/auth/components/OAuthButtons";
import { PasswordInput } from "@/pages/auth/components/PasswordInput";

const loginSchema = z.object({
  email: z
    .string()
    .min(1, "Email is required")
    .email("Enter a valid email address"),
  password: z.string().min(8, "Password must be at least 8 characters"),
});

const OAUTH_ENABLED = import.meta.env.VITE_GOOGLE_AUTH_ENABLED === "true";

type LoginFormValues = z.infer<typeof loginSchema>;

export function LoginCard() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const [authError, setAuthError] = useState<string | null>(null);

  const form = useForm<LoginFormValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: { email: "", password: "" },
  });

  const isBusy = form.formState.isSubmitting;

  const onSubmit = async (values: LoginFormValues) => {
    setAuthError(null);
    try {
      await login(values);
      toast.success("Welcome back");
      navigate("/dashboard");
    } catch (err) {
      if (err instanceof AuthApiError && err.code === "email_verification_required") {
        navigate("/verification-pending", { state: { email: values.email } });
        return;
      }
      setAuthError(err instanceof Error ? err.message : "Sign in failed");
    }
  };

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.18 }}
      className="w-full max-w-md"
    >
      <div className="rounded-2xl border border-zinc-800 bg-zinc-950/80 p-6 shadow-xl shadow-black/30 sm:p-8">
        <Link
          to="/"
          className="mb-6 inline-flex items-center gap-2 text-sm text-zinc-500 transition hover:text-zinc-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to home
        </Link>

        {/* Logo */}
        <div className="mb-6 flex flex-col items-center gap-3">
          <div className="grid h-10 w-10 place-items-center rounded-lg border border-zinc-800 bg-zinc-900/50">
            <ShieldCheck className="h-5 w-5 text-zinc-300" />
          </div>
          <div className="text-center">
            <p className="brand-wordmark mb-3 text-3xl font-semibold text-zinc-50">
              Alarvest
            </p>
            <h1 className="text-xl font-semibold tracking-tight text-zinc-50">
              Welcome back
            </h1>
            <p className="mt-1 text-sm leading-relaxed text-zinc-400">
              Access your private portfolio.
            </p>
          </div>
        </div>

        {OAUTH_ENABLED && (
          <>
            <OAuthButtons
              disabled={isBusy}
              onSuccess={() => navigate("/dashboard")}
              onError={setAuthError}
            />
            <AuthDivider />
          </>
        )}

        {/* Form */}
        <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
          <div className="space-y-4">
            <FloatingLabelInput
              id="email"
              label="Email address"
              type="email"
              autoComplete="email"
              registration={form.register("email")}
              error={form.formState.errors.email?.message}
              disabled={isBusy}
            />
            <PasswordInput
              id="password"
              label="Password"
              autoComplete="current-password"
              registration={form.register("password")}
              error={form.formState.errors.password?.message}
              disabled={isBusy}
            />
            <div className="text-right">
              <Link
                to="/forgot-password"
                className="text-xs font-medium text-violet-300 hover:text-violet-200"
              >
                Forgot password?
              </Link>
            </div>
          </div>

          {/* Auth-level error */}
          <AnimatePresence>
            {authError && (
              <motion.div
                role="alert"
                initial={{ opacity: 0, y: -6 }}
                animate={{ opacity: 1, y: 0 }}
                exit={{ opacity: 0, y: -6 }}
                className="mt-4 rounded-xl border border-rose-400/20 bg-rose-400/10 px-4 py-3 text-sm text-rose-200"
              >
                {authError}
              </motion.div>
            )}
          </AnimatePresence>

          {/* Submit */}
          <button
            type="submit"
            disabled={isBusy}
            className="mt-5 flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-zinc-50 text-sm font-medium text-zinc-950 transition hover:bg-white disabled:cursor-not-allowed disabled:opacity-70"
          >
            {form.formState.isSubmitting ? (
              <>
                <AuthLoadingSpinner />
                Securing session…
              </>
            ) : (
              "Sign In"
            )}
          </button>
        </form>

        {/* Register link */}
        <p className="mt-5 text-center text-sm text-zinc-500">
          Don&apos;t have an account?{" "}
          <Link
            to="/register"
            className="font-medium text-zinc-300 underline underline-offset-2 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400/40"
          >
            Create one
          </Link>
        </p>

        <AuthSecurityNote />
      </div>
    </motion.div>
  );
}
