import {
  LockKeyhole,
  Palette,
  Settings2,
  ShieldCheck,
  Sparkles,
  UserRound,
  WalletCards,
} from "lucide-react";
import { toast } from "sonner";

import { AppNav } from "@/components/layout/AppNav";
import { ProfileForm } from "@/components/profile/ProfileForm";
import { ProfileSkeleton } from "@/components/profile/ProfileSkeleton";
import { BlockedUsersCard } from "@/components/settings/BlockedUsersCard";
import { ChangePasswordCard } from "@/components/settings/ChangePasswordCard";
import { DeleteAccountCard } from "@/components/settings/DeleteAccountCard";
import { PortfolioSettingsCard } from "@/components/settings/PortfolioSettingsCard";
import { useMyProfile, useUpdateProfile } from "@/hooks/useProfile";

export function AccountSettingsPage() {
  const profile = useMyProfile();
  const updateProfile = useUpdateProfile();

  return (
    <div className="min-h-screen bg-[radial-gradient(circle_at_8%_5%,rgba(34,211,238,0.08),transparent_26%),radial-gradient(circle_at_90%_12%,rgba(139,92,246,0.1),transparent_28%),#09090b] text-zinc-50">
      <main className="mx-auto w-full max-w-7xl px-4 pb-16 pt-4 sm:px-6 lg:px-8">
        <AppNav />

        <header className="relative mb-8 overflow-hidden rounded-3xl border border-violet-300/15 bg-gradient-to-br from-cyan-400/[0.08] via-zinc-900/70 to-violet-500/[0.12] px-5 py-6 sm:px-7 sm:py-8">
          <div className="absolute -right-12 -top-14 h-40 w-40 rounded-full bg-violet-500/10 blur-3xl" />
          <div className="relative flex flex-col gap-5 sm:flex-row sm:items-end sm:justify-between">
            <div>
              <div className="mb-3 inline-flex items-center gap-2 rounded-full border border-cyan-300/20 bg-cyan-300/[0.07] px-3 py-1 text-[10px] font-semibold uppercase tracking-[0.2em] text-cyan-200">
                <Settings2 className="h-3.5 w-3.5" />
                Control center
              </div>
              <h1 className="text-3xl font-semibold tracking-tight sm:text-4xl">
                Settings
              </h1>
              <p className="mt-2 max-w-2xl text-sm leading-6 text-zinc-400">
                Shape your public identity, portfolio behavior, security, and
                account preferences from one place.
              </p>
            </div>
            <div className="grid grid-cols-2 gap-2 text-xs sm:flex">
              <StatusPill icon={UserRound} label="Profile" color="cyan" />
              <StatusPill icon={ShieldCheck} label="Security" color="violet" />
            </div>
          </div>
        </header>

        <section className="grid items-start gap-5 lg:grid-cols-12">
          <div className="lg:col-span-7">
            <SectionLabel
              icon={Palette}
              label="Identity & privacy"
              color="cyan"
            />
            {profile.isLoading ? (
              <ProfileSkeleton />
            ) : profile.isError || !profile.data ? (
              <div className="rounded-2xl border border-rose-400/20 bg-rose-400/[0.04] p-6 text-sm text-rose-200">
                Profile settings could not be loaded.
              </div>
            ) : (
              <ProfileForm
                profile={profile.data}
                onSubmit={(input) =>
                  updateProfile.mutate(input, {
                    onSuccess: () => toast.success("Profile settings saved"),
                  })
                }
                isSaving={updateProfile.isPending}
                serverError={updateProfile.error?.message}
              />
            )}
          </div>

          <div className="space-y-5 lg:col-span-5">
            <div>
              <SectionLabel
                icon={WalletCards}
                label="Portfolio behavior"
                color="emerald"
              />
              <PortfolioSettingsCard />
            </div>
            <div>
              <SectionLabel
                icon={LockKeyhole}
                label="Login security"
                color="violet"
              />
              <ChangePasswordCard />
            </div>
            <div>
              <SectionLabel icon={ShieldCheck} label="Blocking" color="cyan" />
              <BlockedUsersCard />
            </div>
          </div>

          <div className="lg:col-span-12">
            <DeleteAccountCard />
          </div>
        </section>
      </main>
    </div>
  );
}

function SectionLabel({
  icon: Icon,
  label,
  color,
}: {
  icon: typeof Sparkles;
  label: string;
  color: "cyan" | "emerald" | "violet";
}) {
  const colors = {
    cyan: "text-cyan-300",
    emerald: "text-emerald-300",
    violet: "text-violet-300",
  };
  return (
    <div className={`mb-2.5 flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.16em] ${colors[color]}`}>
      <Icon className="h-3.5 w-3.5" />
      {label}
    </div>
  );
}

function StatusPill({
  icon: Icon,
  label,
  color,
}: {
  icon: typeof UserRound;
  label: string;
  color: "cyan" | "violet";
}) {
  return (
    <span
      className={
        color === "cyan"
          ? "inline-flex items-center gap-2 rounded-xl border border-cyan-300/15 bg-cyan-300/[0.06] px-3 py-2 text-cyan-100"
          : "inline-flex items-center gap-2 rounded-xl border border-violet-300/15 bg-violet-300/[0.06] px-3 py-2 text-violet-100"
      }
    >
      <Icon className="h-3.5 w-3.5" />
      {label}
    </span>
  );
}
