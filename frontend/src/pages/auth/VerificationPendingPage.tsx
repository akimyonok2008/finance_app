import { MailCheck } from "lucide-react";
import { useState } from "react";
import { Link, useLocation } from "react-router-dom";
import { toast } from "sonner";

import { resendVerificationRequest } from "@/api/authApi";
import { Button } from "@/components/ui/button";

export function VerificationPendingPage() {
  const location = useLocation();
  const email = String((location.state as { email?: string } | null)?.email ?? "");
  const [pending, setPending] = useState(false);

  const resend = async () => {
    setPending(true);
    try {
      await resendVerificationRequest(email);
      toast.success("If verification is available, a new email was sent.");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Unable to resend email");
    } finally {
      setPending(false);
    }
  };

  return (
    <main className="grid min-h-screen place-items-center bg-zinc-950 px-4 text-zinc-50">
      <section className="w-full max-w-lg rounded-3xl border border-cyan-300/15 bg-gradient-to-br from-cyan-400/[0.08] via-zinc-950 to-violet-400/[0.07] p-8 text-center shadow-2xl shadow-cyan-950/20">
        <MailCheck className="mx-auto h-11 w-11 text-cyan-200" />
        <h1 className="mt-5 text-2xl font-semibold">Verify your email</h1>
        <p className="mt-3 text-sm leading-6 text-zinc-400">
          We sent a one-time verification link{email ? ` to ${email}` : ""}.
          Your portfolio session starts only after that link is verified.
        </p>
        <div className="mt-7 grid gap-3 sm:grid-cols-2">
          <Button variant="outline" disabled={pending || !email} onClick={resend}>
            {pending ? "Sending…" : "Resend email"}
          </Button>
          <Button asChild>
            <Link to="/login">Back to sign in</Link>
          </Button>
        </div>
      </section>
    </main>
  );
}
