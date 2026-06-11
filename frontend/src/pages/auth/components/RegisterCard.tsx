import { AnimatePresence, motion } from "framer-motion";
import { ShieldCheck } from "lucide-react";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { useNavigate } from "react-router-dom";

import { registerRequest } from "@/api/authApi";
import { useAuth } from "@/auth/useAuth";
import { AuthLoadingSpinner } from "@/pages/auth/components/AuthLoadingSpinner";
import { AuthSecurityNote } from "@/pages/auth/components/AuthSecurityNote";
import { FloatingLabelInput } from "@/pages/auth/components/FloatingLabelInput";
import { PasswordInput } from "@/pages/auth/components/PasswordInput";
import type { LoginFormValues } from "@/types/auth";

const registerSchema = z.object({
  display_name: z
    .string()
    .min(2, "Display name must be at least 2 characters")
    .max(32, "Display name must be 32 characters or fewer"),
  email: z
    .string()
    .min(1, "Email is required")
    .email("Enter a valid email address"),
  password: z.string().min(8, "Password must be at least 8 characters"),
});

type RegisterFormValues = z.infer<typeof registerSchema>;

export function RegisterCard() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const [authError, setAuthError] = useState<string | null>(null);

  const form = useForm<RegisterFormValues>({
    resolver: zodResolver(registerSchema),
    defaultValues: { display_name: "", email: "", password: "" },
  });

  const isBusy = form.formState.isSubmitting;

  const onSubmit = async (values: RegisterFormValues) => {
    setAuthError(null);
    try {
      // Register via backend, then fall through to login to persist session.
      await registerRequest({
        email: values.email,
        password: values.password,
        display_name: values.display_name,
      });
      // Login reuses AuthProvider's login (stores token + user in localStorage).
      await login({ email: values.email, password: values.password } as LoginFormValues);
      toast.success("Account created — welcome!");
      navigate("/dashboard");
    } catch (err) {
      setAuthError(err instanceof Error ? err.message : "Registration failed");
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

        {/* Logo */}
        <div className="mb-6 flex flex-col items-center gap-3">
          <div className="grid h-10 w-10 place-items-center rounded-lg border border-zinc-800 bg-zinc-900/50">
            <ShieldCheck className="h-5 w-5 text-zinc-300" />
          </div>
          <div className="text-center">
            <h1 className="text-xl font-semibold tracking-tight text-zinc-50">
              Create your account
            </h1>
            <p className="mt-1 text-sm leading-relaxed text-zinc-400">
              Create your private portfolio.
            </p>
          </div>
        </div>

        <form onSubmit={form.handleSubmit(onSubmit)} noValidate>
          <div className="space-y-4">
            <FloatingLabelInput
              id="display_name"
              label="Display name"
              autoComplete="nickname"
              registration={form.register("display_name")}
              error={form.formState.errors.display_name?.message}
              disabled={isBusy}
            />
            <FloatingLabelInput
              id="reg-email"
              label="Email address"
              type="email"
              autoComplete="email"
              registration={form.register("email")}
              error={form.formState.errors.email?.message}
              disabled={isBusy}
            />
            <PasswordInput
              id="reg-password"
              label="Password"
              autoComplete="new-password"
              registration={form.register("password")}
              error={form.formState.errors.password?.message}
              disabled={isBusy}
            />
          </div>

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

          <button
            type="submit"
            disabled={isBusy}
            className="mt-5 flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-zinc-50 text-sm font-medium text-zinc-950 transition hover:bg-white disabled:cursor-not-allowed disabled:opacity-70"
          >
            {isBusy ? (
              <>
                <AuthLoadingSpinner />
                Creating account…
              </>
            ) : (
              "Create Account"
            )}
          </button>
        </form>

        <p className="mt-5 text-center text-sm text-zinc-500">
          Already have an account?{" "}
          <a
            href="/login"
            className="font-medium text-zinc-300 underline underline-offset-2 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-violet-400/40"
          >
            Sign in
          </a>
        </p>

        <AuthSecurityNote />
      </div>
    </motion.div>
  );
}
