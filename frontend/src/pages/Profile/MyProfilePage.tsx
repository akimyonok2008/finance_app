import { useState } from "react";
import { ArrowLeft, LockKeyhole, RefreshCw, Settings } from "lucide-react";

import { AppNav } from "@/components/layout/AppNav";
import { ProfileForm } from "@/components/profile/ProfileForm";
import { ProfileSkeleton } from "@/components/profile/ProfileSkeleton";
import { PublicProfileDisplay } from "@/components/profile/PublicProfileDisplay";
import { Button } from "@/components/ui/button";
import { useMyProfile, useUpdateProfile } from "@/hooks/useProfile";

export function MyProfilePage() {
  const query = useMyProfile();
  const mutation = useUpdateProfile();
  const [settingsOpen, setSettingsOpen] = useState(false);

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50">
      <main className="mx-auto w-full max-w-7xl px-4 pb-16 pt-4 sm:px-6 lg:px-8">
        <AppNav />
        <header className="mb-8 flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <h1 className="text-2xl font-medium tracking-tight sm:text-3xl">
              {settingsOpen ? "Profile settings" : "My Profile"}
            </h1>
            <p className="mt-2 text-sm text-zinc-400">
              {settingsOpen
                ? "Choose what appears on your public strategy profile."
                : "Preview your profile as other users see it."}
            </p>
          </div>
          {query.data ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => setSettingsOpen((open) => !open)}
              disabled={mutation.isPending}
            >
              {settingsOpen ? <ArrowLeft /> : <Settings />}
              {settingsOpen ? "Back to preview" : "Settings"}
            </Button>
          ) : null}
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
        ) : settingsOpen ? (
          <div className="mx-auto w-full max-w-xl">
            <ProfileForm
              profile={query.data}
              onSubmit={(input) =>
                mutation.mutate(input, {
                  onSuccess: () => setSettingsOpen(false),
                })
              }
              isSaving={mutation.isPending}
              serverError={mutation.error?.message}
            />
          </div>
        ) : (
          <section className="min-w-0 space-y-5">
            {!query.data.is_public ? (
              <div className="flex items-start gap-3 rounded-xl border border-zinc-800 bg-zinc-900/40 px-4 py-3">
                <LockKeyhole className="mt-0.5 h-4 w-4 shrink-0 text-zinc-500" />
                <div>
                  <h2 className="text-sm font-medium text-zinc-200">
                    Your profile is private
                  </h2>
                  <p className="mt-1 text-xs leading-5 text-zinc-500">
                    This preview is visible only to you. Open Settings to make
                    the profile public.
                  </p>
                </div>
              </div>
            ) : null}
            <PublicProfileDisplay profile={query.data.public_preview} />
          </section>
        )}
      </main>
    </div>
  );
}
