import { LockKeyhole, RefreshCw } from "lucide-react";

import { AppNav } from "@/components/layout/AppNav";
import { ProfileForm } from "@/components/profile/ProfileForm";
import { ProfileSkeleton } from "@/components/profile/ProfileSkeleton";
import { PublicProfileDisplay } from "@/components/profile/PublicProfileDisplay";
import { Button } from "@/components/ui/button";
import { useMyProfile, useUpdateProfile } from "@/hooks/useProfile";

export function MyProfilePage() {
  const query = useMyProfile();
  const mutation = useUpdateProfile();

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50">
      <main className="mx-auto w-full max-w-7xl px-4 pb-16 pt-4 sm:px-6 lg:px-8">
        <AppNav />
        <header className="mb-8">
          <h1 className="text-2xl font-medium tracking-tight sm:text-3xl">My Profile</h1>
          <p className="mt-2 text-sm text-zinc-400">Control how your public strategy profile appears.</p>
        </header>

        {query.isLoading ? (
          <ProfileSkeleton />
        ) : query.isError || !query.data ? (
          <div className="rounded-2xl border border-rose-400/15 bg-rose-400/[0.04] px-6 py-14 text-center">
            <h2 className="text-lg font-semibold">Could not load your profile.</h2>
            <Button variant="outline" className="mt-5" onClick={() => query.refetch()}>
              <RefreshCw /> Retry
            </Button>
          </div>
        ) : (
          <div className="grid items-start gap-6 xl:grid-cols-[380px_1fr]">
            <ProfileForm
              profile={query.data}
              onSubmit={(input) => mutation.mutate(input)}
              isSaving={mutation.isPending}
              serverError={mutation.error?.message}
            />
            <section className="min-w-0">
              <div className="mb-4">
                <h2 className="text-sm font-semibold text-zinc-100">Public preview</h2>
                <p className="mt-1 text-xs text-zinc-500">This is what other users can see.</p>
              </div>
              {!query.data.is_public ? (
                <div className="rounded-2xl border border-zinc-800 bg-zinc-900/40 px-6 py-16 text-center">
                  <LockKeyhole className="mx-auto h-6 w-6 text-zinc-500" />
                  <h3 className="mt-4 text-base font-semibold text-zinc-100">Your profile is private.</h3>
                  <p className="mt-2 text-sm text-zinc-500">Other users cannot view it.</p>
                </div>
              ) : (
                <PublicProfileDisplay profile={query.data.public_preview} />
              )}
            </section>
          </div>
        )}
      </main>
    </div>
  );
}
