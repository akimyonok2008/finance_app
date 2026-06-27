import { RefreshCw } from "lucide-react";
import { useParams } from "react-router-dom";

import { ApiError } from "@/api/client";
import { AppNav } from "@/components/layout/AppNav";
import { ProfileNotFoundState } from "@/components/profile/ProfileNotFoundState";
import { ProfileSkeleton } from "@/components/profile/ProfileSkeleton";
import { PublicProfileDisplay } from "@/components/profile/PublicProfileDisplay";
import { Button } from "@/components/ui/button";
import { usePublicProfile } from "@/hooks/useProfile";

export function PublicProfilePage() {
  const { handle = "" } = useParams();
  const query = usePublicProfile(handle);
  const notFound = query.error instanceof ApiError && query.error.status === 404;

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50">
      <main className="mx-auto w-full max-w-6xl px-4 pb-16 pt-4 sm:px-6 lg:px-8">
        <AppNav />
        {query.isLoading ? (
          <ProfileSkeleton />
        ) : notFound ? (
          <ProfileNotFoundState />
        ) : query.isError || !query.data ? (
          <div className="rounded-2xl border border-rose-400/15 bg-rose-400/[0.04] px-6 py-14 text-center">
            <h1 className="text-xl font-semibold">Profile could not be loaded.</h1>
            <p className="mt-2 text-sm text-zinc-500">Please try again in a moment.</p>
            <Button variant="outline" className="mt-5" onClick={() => query.refetch()}>
              <RefreshCw /> Retry
            </Button>
          </div>
        ) : (
          <PublicProfileDisplay profile={query.data} />
        )}
      </main>
    </div>
  );
}
