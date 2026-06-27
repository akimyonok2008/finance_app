import { Link } from "react-router-dom";

import { Button } from "@/components/ui/button";

export function ProfileNotFoundState() {
  return (
    <div className="rounded-2xl border border-zinc-800 bg-zinc-900/40 px-6 py-16 text-center">
      <h1 className="text-xl font-semibold text-zinc-100">Profile not found</h1>
      <p className="mt-2 text-sm text-zinc-500">This profile may be private or unavailable.</p>
      <Button asChild variant="outline" className="mt-6">
        <Link to="/dashboard">Back to Dashboard</Link>
      </Button>
    </div>
  );
}
