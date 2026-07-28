import { ArrowLeft, ShieldCheck } from "lucide-react";
import { Link } from "react-router-dom";

const NEVER_PUBLIC = [
  "Position quantities or units",
  "Cash, account, or portfolio monetary values",
  "Purchase, sale, execution, or average prices",
  "Cost basis, proceeds, fees, or absolute gain/loss",
  "Email, authentication data, brokerage identifiers, or internal IDs",
];

export function PrivacyPolicyPage() {
  return (
    <main className="min-h-screen bg-zinc-950 px-4 py-10 text-zinc-50 sm:px-6">
      <article className="mx-auto max-w-3xl">
        <Link
          to="/"
          className="inline-flex items-center gap-2 text-sm text-zinc-400 transition hover:text-zinc-100"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to Alarvest
        </Link>

        <header className="mt-10 border-b border-zinc-800 pb-8">
          <div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.18em] text-cyan-300">
            <ShieldCheck className="h-4 w-4" />
            Product data visibility
          </div>
          <h1 className="mt-4 text-4xl font-semibold tracking-tight">
            Privacy Policy
          </h1>
          <p className="mt-3 text-sm text-zinc-500">Effective July 28, 2026</p>
          <p className="mt-6 text-base leading-7 text-zinc-300">
            Alarvest protects monetary account details while allowing
            percentage-based strategy sharing. Holding symbols and allocation
            percentages become public strategy information only when you
            explicitly enable composition sharing.
          </p>
        </header>

        <div className="space-y-10 py-10 text-sm leading-7 text-zinc-300">
          <section>
            <h2 className="text-xl font-semibold text-zinc-100">
              What your controls mean
            </h2>
            <div className="mt-4 space-y-4">
              <p>
                Profiles and composition sharing start off. Enabling{" "}
                <strong className="text-zinc-100">Public profile</strong> makes
                your profile identity, ranked performance, and non-monetary
                strategy analytics visible to other users.
              </p>
              <p>
                <strong className="text-zinc-100">Show public weights</strong>{" "}
                is a separate opt-in. When both controls are enabled, Alarvest
                may show active and closed symbols, asset types, allocation
                percentages, symbol-level performance drivers, and reusable
                Compare or Copy weights. This composition may also appear in
                Explore, trending, similar-strategy, and leaderboard features.
              </p>
              <p>
                Turning either control off stops subsequent product responses
                from disclosing the affected profile or composition. It cannot
                recall information another person already saw or copied.
              </p>
            </div>
          </section>

          <section>
            <h2 className="text-xl font-semibold text-zinc-100">
              Competitive visibility
            </h2>
            <p className="mt-4">
              Eligible rankings may show your display name, avatar, rank,
              ranked index, and percentage return even when your profile is
              private. A private profile does not expose its handle, profile
              page, or composition.
            </p>
          </section>

          <section>
            <h2 className="text-xl font-semibold text-zinc-100">
              What always stays private
            </h2>
            <ul className="mt-4 grid gap-3 sm:grid-cols-2">
              {NEVER_PUBLIC.map((item) => (
                <li
                  key={item}
                  className="rounded-xl border border-zinc-800 bg-zinc-900/40 px-4 py-3"
                >
                  {item}
                </li>
              ))}
            </ul>
          </section>

          <section className="rounded-2xl border border-cyan-300/15 bg-cyan-300/[0.045] p-5">
            <h2 className="font-semibold text-cyan-100">Benchmark symbols</h2>
            <p className="mt-2 text-zinc-400">
              A benchmark symbol identifies a public reference benchmark. It
              does not state or imply that you own that instrument.
            </p>
          </section>

          <p className="border-t border-zinc-800 pt-8 text-xs leading-6 text-zinc-500">
            This page describes product visibility boundaries. It is not a
            jurisdiction-specific legal notice covering processors, retention,
            or data-subject rights.
          </p>
        </div>
      </article>
    </main>
  );
}
