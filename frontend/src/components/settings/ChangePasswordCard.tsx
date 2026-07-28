import { zodResolver } from "@hookform/resolvers/zod";
import { LoaderCircle } from "lucide-react";
import { useState } from "react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { reauthenticateWithProviderRequest } from "@/api/accountApi";
import { useAuth } from "@/auth/useAuth";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useChangePassword, useSetFirstPassword } from "@/hooks/useAccount";
import { GoogleSignInButton } from "@/pages/auth/components/OAuthButtons";

const schema = z
  .object({
    current_password: z.string().min(1, "Current password is required"),
    new_password: z.string().min(8, "New password must be at least 8 characters"),
    confirm_password: z.string().min(1, "Confirm your new password"),
  })
  .refine((values) => values.new_password === values.confirm_password, {
    message: "Passwords do not match",
    path: ["confirm_password"],
  });

type Values = z.infer<typeof schema>;

function FieldError({ message }: { message?: string }) {
  return message ? <p className="mt-1.5 text-xs text-rose-300">{message}</p> : null;
}

export function ChangePasswordCard() {
  const { user } = useAuth();
  if (user && !user.has_password) {
    return <SetFirstPasswordCard />;
  }
  return <ExistingPasswordCard />;
}

function ExistingPasswordCard() {
  const changePassword = useChangePassword();
  const {
    register,
    handleSubmit,
    reset,
    setError,
    formState: { errors },
  } = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { current_password: "", new_password: "", confirm_password: "" },
  });

  const onSubmit = (values: Values) => {
    changePassword.mutate(
      { currentPassword: values.current_password, newPassword: values.new_password },
      {
        onSuccess: () => reset(),
        onError: (err: Error) => {
          setError("current_password", { message: err.message });
        },
      },
    );
  };

  return (
    <form
      onSubmit={handleSubmit(onSubmit)}
      className="rounded-2xl border border-violet-300/15 bg-gradient-to-br from-violet-400/[0.055] to-zinc-900/70 p-5 shadow-lg shadow-violet-950/10 sm:p-6"
    >
      <h2 className="text-base font-semibold text-zinc-50">Change password</h2>
      <p className="mt-1 text-xs text-zinc-400">
        You&apos;ll need your current password to set a new one.
      </p>

      <div className="mt-5 grid gap-4 sm:grid-cols-2">
        <div className="sm:col-span-2">
          <Label htmlFor="current_password">Current password</Label>
          <Input
            id="current_password"
            type="password"
            autoComplete="current-password"
            className="mt-2"
            aria-invalid={!!errors.current_password}
            {...register("current_password")}
          />
          <FieldError message={errors.current_password?.message} />
        </div>
        <div>
          <Label htmlFor="new_password">New password</Label>
          <Input
            id="new_password"
            type="password"
            autoComplete="new-password"
            className="mt-2"
            aria-invalid={!!errors.new_password}
            {...register("new_password")}
          />
          <FieldError message={errors.new_password?.message} />
        </div>
        <div>
          <Label htmlFor="confirm_password">Confirm new password</Label>
          <Input
            id="confirm_password"
            type="password"
            autoComplete="new-password"
            className="mt-2"
            aria-invalid={!!errors.confirm_password}
            {...register("confirm_password")}
          />
          <FieldError message={errors.confirm_password?.message} />
        </div>
      </div>

      <Button type="submit" className="mt-5 w-full bg-violet-200 text-zinc-950 hover:bg-violet-100" disabled={changePassword.isPending}>
        {changePassword.isPending ? <LoaderCircle className="animate-spin" /> : null}
        {changePassword.isPending ? "Updating password" : "Update password"}
      </Button>
    </form>
  );
}

function SetFirstPasswordCard() {
  const setPassword = useSetFirstPassword();
  const [reauthToken, setReauthToken] = useState("");
  const [googleBusy, setGoogleBusy] = useState(false);
  const [googleError, setGoogleError] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirm, setConfirm] = useState("");

  const submit = (event: React.FormEvent) => {
    event.preventDefault();
    if (newPassword.length < 8 || newPassword !== confirm) return;
    setPassword.mutate(
      { reauthenticationToken: reauthToken, newPassword },
      { onSuccess: () => {
        setNewPassword("");
        setConfirm("");
        setReauthToken("");
      } },
    );
  };

  return (
    <form onSubmit={submit} className="rounded-2xl border border-violet-300/15 bg-gradient-to-br from-violet-400/[0.055] to-zinc-900/70 p-5 shadow-lg shadow-violet-950/10 sm:p-6">
      <h2 className="text-base font-semibold text-zinc-50">Create a password</h2>
      <p className="mt-1 text-xs leading-5 text-zinc-400">
        This account currently uses Google. Reauthenticate with the linked
        identity before adding an optional password.
      </p>
      {!reauthToken ? (
        <div className="mt-5">
          <GoogleSignInButton
            loading={googleBusy}
            disabled={googleBusy}
            onStart={() => setGoogleBusy(true)}
            onDone={() => setGoogleBusy(false)}
            onSuccess={() => undefined}
            onError={setGoogleError}
            loginWithGoogle={async (credential) => {
              const result = await reauthenticateWithProviderRequest("google", credential);
              setReauthToken(result.reauthentication_token);
              setGoogleError("");
            }}
          />
          {googleError ? <p className="mt-2 text-xs text-rose-300">{googleError}</p> : null}
        </div>
      ) : (
        <>
          <div className="mt-5 grid gap-4 sm:grid-cols-2">
            <div>
              <Label htmlFor="provider_new_password">New password</Label>
              <Input id="provider_new_password" className="mt-2" type="password" autoComplete="new-password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} />
            </div>
            <div>
              <Label htmlFor="provider_confirm_password">Confirm password</Label>
              <Input id="provider_confirm_password" className="mt-2" type="password" autoComplete="new-password" value={confirm} onChange={(event) => setConfirm(event.target.value)} />
            </div>
          </div>
          {newPassword && (newPassword.length < 8 || newPassword !== confirm) ? (
            <p className="mt-2 text-xs text-rose-300">
              {newPassword.length < 8 ? "Password must be at least 8 characters." : "Passwords do not match."}
            </p>
          ) : null}
          <Button className="mt-5 w-full bg-violet-200 text-zinc-950 hover:bg-violet-100" disabled={setPassword.isPending || newPassword.length < 8 || newPassword !== confirm}>
            {setPassword.isPending ? "Creating password…" : "Create password"}
          </Button>
        </>
      )}
    </form>
  );
}
