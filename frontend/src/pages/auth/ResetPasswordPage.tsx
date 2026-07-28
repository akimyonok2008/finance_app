import { ShieldCheck } from "lucide-react";
import { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";

import { resetPasswordRequest } from "@/api/authApi";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function ResetPasswordPage() {
  const [params] = useSearchParams();
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [pending, setPending] = useState(false);
  const [complete, setComplete] = useState(false);
  const [error, setError] = useState("");

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (password.length < 8 || password !== confirm) {
      setError(password.length < 8 ? "Password must be at least 8 characters." : "Passwords do not match.");
      return;
    }
    setPending(true);
    setError("");
    try {
      await resetPasswordRequest(params.get("token") ?? "", password);
      setComplete(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unable to reset password");
    } finally {
      setPending(false);
    }
  };

  return (
    <main className="grid min-h-screen place-items-center bg-zinc-950 px-4 text-zinc-50">
      <form onSubmit={submit} className="w-full max-w-md rounded-3xl border border-cyan-300/15 bg-zinc-900/70 p-8">
        <ShieldCheck className="h-9 w-9 text-cyan-200" />
        <h1 className="mt-5 text-2xl font-semibold">Choose a new password</h1>
        {complete ? (
          <>
            <p role="status" className="mt-4 text-sm text-emerald-200">Password updated. Older sessions have been revoked.</p>
            <Button className="mt-6 w-full" asChild><Link to="/login">Sign in</Link></Button>
          </>
        ) : (
          <>
            <div className="mt-6 space-y-4">
              <div><Label htmlFor="new-password">New password</Label><Input id="new-password" className="mt-2" type="password" autoComplete="new-password" value={password} onChange={(event) => setPassword(event.target.value)} /></div>
              <div><Label htmlFor="confirm-password">Confirm password</Label><Input id="confirm-password" className="mt-2" type="password" autoComplete="new-password" value={confirm} onChange={(event) => setConfirm(event.target.value)} /></div>
            </div>
            {error ? <p role="alert" className="mt-3 text-xs text-rose-300">{error}</p> : null}
            <Button className="mt-5 w-full" disabled={pending || !params.get("token")}>{pending ? "Updating…" : "Update password"}</Button>
          </>
        )}
      </form>
    </main>
  );
}
