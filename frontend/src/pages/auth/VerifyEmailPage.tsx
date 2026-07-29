import { CircleCheck, CircleX, LoaderCircle } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";

import { verifyEmailRequest } from "@/api/authApi";
import { useAuth } from "@/auth/useAuth";
import { useScrubbedURLToken } from "@/auth/useScrubbedURLToken";
import { Button } from "@/components/ui/button";

type State = "loading" | "success" | "error";

export function VerifyEmailPage() {
  const token = useScrubbedURLToken();
  const { acceptSession } = useAuth();
  const [state, setState] = useState<State>(token ? "loading" : "error");
  const [message, setMessage] = useState(token ? "" : "This verification link is incomplete.");
  const started = useRef(false);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    if (!token) return;
    void verifyEmailRequest(token)
      .then((session) => {
        acceptSession(session);
        setState("success");
      })
      .catch((err: unknown) => {
        setState("error");
        setMessage(err instanceof Error ? err.message : "Verification failed.");
      });
  }, [acceptSession, token]);

  return (
    <main className="grid min-h-screen place-items-center bg-zinc-950 px-4 text-zinc-50">
      <section className="w-full max-w-lg rounded-3xl border border-zinc-800 bg-zinc-900/70 p-8 text-center">
        {state === "loading" ? <LoaderCircle className="mx-auto h-10 w-10 animate-spin text-cyan-200" /> : null}
        {state === "success" ? <CircleCheck className="mx-auto h-10 w-10 text-emerald-300" /> : null}
        {state === "error" ? <CircleX className="mx-auto h-10 w-10 text-rose-300" /> : null}
        <h1 className="mt-5 text-2xl font-semibold">
          {state === "loading" ? "Verifying your email" : state === "success" ? "Email verified" : "Link unavailable"}
        </h1>
        <p className="mt-3 text-sm text-zinc-400">
          {state === "loading"
            ? "Checking your one-time link…"
            : state === "success"
              ? "Your account is active and your secure session is ready."
              : message}
        </p>
        {state !== "loading" ? (
          <Button className="mt-7" asChild>
            <Link to={state === "success" ? "/dashboard" : "/login"}>
              {state === "success" ? "Open dashboard" : "Back to sign in"}
            </Link>
          </Button>
        ) : null}
      </section>
    </main>
  );
}
