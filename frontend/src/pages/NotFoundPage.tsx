import { ArrowLeft } from "lucide-react";
import { Link } from "react-router-dom";

export function NotFoundPage() {
  return (
    <main className="flex min-h-screen items-center justify-center bg-zinc-950 px-4 text-zinc-50">
      <div className="text-center">
        <p className="text-sm font-semibold uppercase tracking-[0.18em] text-cyan-300">
          404
        </p>
        <h1 className="mt-4 text-3xl font-semibold tracking-tight">
          Page not found
        </h1>
        <p className="mt-2 text-sm text-zinc-400">
          The page you're looking for doesn't exist or may have been moved.
        </p>
        <Link
          to="/"
          className="mt-8 inline-flex items-center gap-2 text-sm text-zinc-300 transition hover:text-zinc-100"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to Alarvest
        </Link>
      </div>
    </main>
  );
}
