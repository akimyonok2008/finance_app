import { KeyRound } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";

import { forgotPasswordRequest } from "@/api/authApi";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function ForgotPasswordPage() {
  const [email, setEmail] = useState("");
  const [sent, setSent] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    setPending(true);
    setError("");
    try {
      await forgotPasswordRequest(email);
      setSent(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to request reset");
    } finally {
      setPending(false);
    }
  };

  return (
    <main className="grid min-h-screen place-items-center bg-zinc-950 px-4 text-zinc-50">
      <form onSubmit={submit} className="w-full max-w-md rounded-3xl border border-violet-300/15 bg-gradient-to-br from-violet-400/[0.08] to-zinc-950 p-8">
        <KeyRound className="h-9 w-9 text-violet-200" />
        <h1 className="mt-5 text-2xl font-semibold">Reset your password</h1>
        <p className="mt-2 text-sm leading-6 text-zinc-400">
          Enter your email. The response is always the same to protect account privacy.
        </p>
        {sent ? (
          <div role="status" className="mt-6 rounded-xl border border-emerald-300/20 bg-emerald-400/10 p-4 text-sm text-emerald-100">
            If the account can be recovered, a reset email has been sent.
          </div>
        ) : (
          <div className="mt-6">
            <Label htmlFor="recovery-email">Email address</Label>
            <Input id="recovery-email" className="mt-2" type="email" required value={email} onChange={(event) => setEmail(event.target.value)} />
            {error ? <p role="alert" className="mt-2 text-xs text-rose-300">{error}</p> : null}
            <Button className="mt-5 w-full" disabled={pending}>
              {pending ? "Sending…" : "Send reset link"}
            </Button>
          </div>
        )}
        <Link className="mt-6 block text-center text-sm text-zinc-400 hover:text-zinc-200" to="/login">
          Back to sign in
        </Link>
      </form>
    </main>
  );
}
