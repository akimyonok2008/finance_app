import { zodResolver } from "@hookform/resolvers/zod";
import { LoaderCircle } from "lucide-react";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { STRATEGY_TAGS, type MyProfile, type UpdateProfileRequest } from "@/types/profile";
import { cn } from "@/utils/cn";

const schema = z.object({
  display_name: z.string().trim().min(2, "Use at least 2 characters").max(40, "Use at most 40 characters"),
  handle: z.string().trim().min(3, "Use at least 3 characters").max(30, "Use at most 30 characters").regex(/^[a-z0-9_-]+$/, "Use lower-case letters, numbers, _ or - only"),
  avatar_key: z.string().trim().max(40, "Use at most 40 characters"),
  bio: z.string().trim().max(160, "Use at most 160 characters"),
  strategy_tag: z.enum(STRATEGY_TAGS),
  is_public: z.boolean(),
  show_public_weights: z.boolean(),
});

type Values = z.infer<typeof schema>;

function FieldError({ message }: { message?: string }) {
  return message ? <p className="mt-1.5 text-xs text-rose-300">{message}</p> : null;
}

function Toggle({ checked, onChange, label, description }: { checked: boolean; onChange: (value: boolean) => void; label: string; description: string }) {
  return (
    <div className="flex items-start justify-between gap-4 rounded-xl border border-zinc-800 bg-zinc-950/50 p-4">
      <div>
        <div className="text-sm font-medium text-zinc-200">{label}</div>
        <p className="mt-1 text-xs leading-5 text-zinc-500">{description}</p>
      </div>
      <button
        type="button"
        role="switch"
        aria-checked={checked}
        aria-label={label}
        onClick={() => onChange(!checked)}
        className={cn(
          "relative mt-0.5 h-6 w-11 shrink-0 rounded-full border transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500",
          checked ? "border-zinc-400 bg-zinc-200" : "border-zinc-700 bg-zinc-900",
        )}
      >
        <span className={cn("absolute top-0.5 h-4 w-4 rounded-full transition", checked ? "left-5 bg-zinc-950" : "left-1 bg-zinc-500")} />
      </button>
    </div>
  );
}

export function ProfileForm({ profile, onSubmit, isSaving, serverError }: { profile: MyProfile; onSubmit: (input: UpdateProfileRequest) => void; isSaving: boolean; serverError?: string }) {
  const { register, control, handleSubmit, formState: { errors, isDirty } } = useForm<Values>({
    resolver: zodResolver(schema),
    values: {
      display_name: profile.display_name,
      handle: profile.handle,
      avatar_key: profile.avatar_key ?? "",
      bio: profile.bio ?? "",
      strategy_tag: STRATEGY_TAGS.includes(profile.strategy_tag as (typeof STRATEGY_TAGS)[number])
        ? (profile.strategy_tag as (typeof STRATEGY_TAGS)[number])
        : "balanced_global",
      is_public: profile.is_public,
      show_public_weights: profile.show_public_weights,
    },
  });

  return (
    <form onSubmit={handleSubmit(onSubmit)} className="rounded-2xl border border-zinc-800 bg-zinc-900/50 p-5 sm:p-6">
      <h2 className="text-sm font-semibold text-zinc-100">Profile settings</h2>
      <p className="mt-1 text-xs text-zinc-500">Shape your public strategy identity.</p>

      <div className="mt-6 space-y-5">
        <div>
          <Label htmlFor="display_name">Display name</Label>
          <Input id="display_name" className="mt-2" aria-invalid={!!errors.display_name} {...register("display_name")} />
          <FieldError message={errors.display_name?.message} />
        </div>
        <div>
          <Label htmlFor="handle">Handle</Label>
          <Input id="handle" className="mt-2 font-mono" autoCapitalize="none" aria-invalid={!!errors.handle} {...register("handle")} />
          <p className="mt-1.5 text-xs text-zinc-600">Used in your public profile URL.</p>
          <FieldError message={errors.handle?.message} />
        </div>
        <div>
          <Label htmlFor="avatar_key">Avatar key</Label>
          <Input id="avatar_key" className="mt-2" aria-invalid={!!errors.avatar_key} {...register("avatar_key")} />
          <FieldError message={errors.avatar_key?.message} />
        </div>
        <div>
          <Label htmlFor="bio">Bio</Label>
          <textarea id="bio" rows={4} className="mt-2 flex w-full resize-none rounded-lg border border-zinc-800 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none transition placeholder:text-zinc-600 focus-visible:border-zinc-500 focus-visible:ring-1 focus-visible:ring-zinc-500 aria-[invalid=true]:border-rose-400/60" aria-invalid={!!errors.bio} {...register("bio")} />
          <FieldError message={errors.bio?.message} />
        </div>
        <div>
          <Label>Strategy tag</Label>
          <Controller
            control={control}
            name="strategy_tag"
            render={({ field }) => (
              <Select value={field.value} onValueChange={field.onChange}>
                <SelectTrigger className="mt-2"><SelectValue /></SelectTrigger>
                <SelectContent>
                  {STRATEGY_TAGS.map((tag) => <SelectItem key={tag} value={tag}>{tag.replaceAll("_", " ")}</SelectItem>)}
                </SelectContent>
              </Select>
            )}
          />
        </div>
        <Controller control={control} name="is_public" render={({ field }) => <Toggle checked={field.value} onChange={field.onChange} label="Public profile" description="When off, other users cannot view your profile." />} />
        <Controller control={control} name="show_public_weights" render={({ field }) => <Toggle checked={field.value} onChange={field.onChange} label="Show public weights" description="Others can see symbols and percentage weights, never quantities or values." />} />
      </div>

      {serverError ? <p role="alert" className="mt-5 text-sm text-rose-300">{serverError}</p> : null}
      <Button type="submit" className="mt-6 w-full" disabled={isSaving || !isDirty}>
        {isSaving ? <LoaderCircle className="animate-spin" /> : null}
        {isSaving ? "Saving profile" : "Save profile"}
      </Button>
    </form>
  );
}
