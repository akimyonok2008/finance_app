import { Loader2 } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useAuth } from "@/auth/useAuth";
import { useDeleteAccount } from "@/hooks/useAccount";
import {
  deleteAccountRequest,
  reauthenticateWithProviderRequest,
} from "@/api/accountApi";
import { GoogleSignInButton } from "@/pages/auth/components/OAuthButtons";

export function DeleteAccountCard() {
  const [open, setOpen] = useState(false);
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [providerBusy, setProviderBusy] = useState(false);
  const deleteAccount = useDeleteAccount();
  const { logout, user } = useAuth();
  const navigate = useNavigate();

  const handleOpenChange = (next: boolean) => {
    setOpen(next);
    if (!next) {
      setPassword("");
      setError(null);
    }
  };

  const finishDeletion = () => {
    toast.success("Account deleted");
    logout();
    navigate("/login");
  };

  const deleteWithProvider = async (credential: string) => {
    const reauth = await reauthenticateWithProviderRequest("google", credential);
    await deleteAccountRequest(reauth.reauthentication_token);
    finishDeletion();
  };

  const handleConfirm = (e: React.MouseEvent) => {
    // Keep the dialog open until the request resolves, so a wrong password
    // can be corrected without re-opening the whole flow.
    e.preventDefault();
    setError(null);
    deleteAccount.mutate(password, {
      onSuccess: () => {
        finishDeletion();
      },
      onError: (err: Error) => setError(err.message),
    });
  };

  return (
    <div className="flex flex-col gap-5 rounded-2xl border border-rose-400/20 bg-gradient-to-r from-rose-500/[0.055] to-zinc-900/60 p-5 sm:p-6 lg:flex-row lg:items-center lg:justify-between">
      <div className="max-w-3xl">
      <h2 className="text-sm font-semibold text-rose-200">Danger zone</h2>
      <p className="mt-1 text-xs leading-5 text-zinc-400">
        Deleting your account signs you out everywhere immediately, removes
        your public profile from Explore and search, and cannot be undone from
        the app. This is manual portfolio-tracking data, not brokerage
        history — nothing is bought or sold as part of deletion.
      </p>

      </div>
      <Button
        type="button"
        variant="destructive"
        className="shrink-0"
        onClick={() => setOpen(true)}
      >
        Delete account
      </Button>

      <AlertDialog open={open} onOpenChange={handleOpenChange}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete your account?</AlertDialogTitle>
            <AlertDialogDescription>
              {user?.has_password
                ? "Enter your password to confirm. This cannot be undone."
                : "Confirm with the Google identity linked to this account. This cannot be undone."}
            </AlertDialogDescription>
          </AlertDialogHeader>

          {user?.has_password ? <div>
            <Label htmlFor="delete_account_password">Password</Label>
            <Input
              id="delete_account_password"
              type="password"
              autoComplete="current-password"
              className="mt-2"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              aria-invalid={!!error}
            />
            {error ? <p className="mt-1.5 text-xs text-rose-300">{error}</p> : null}
          </div> : (
            <div>
              <GoogleSignInButton
                loading={providerBusy}
                disabled={providerBusy}
                onStart={() => setProviderBusy(true)}
                onDone={() => setProviderBusy(false)}
                onSuccess={() => undefined}
                onError={setError}
                loginWithGoogle={deleteWithProvider}
              />
              {error ? <p className="mt-1.5 text-xs text-rose-300">{error}</p> : null}
            </div>
          )}

          <AlertDialogFooter>
            <AlertDialogCancel disabled={deleteAccount.isPending}>Cancel</AlertDialogCancel>
            {user?.has_password ? <AlertDialogAction
              onClick={handleConfirm}
              disabled={deleteAccount.isPending || password.length === 0}
            >
              {deleteAccount.isPending ? (
                <>
                  <Loader2 className="animate-spin" />
                  Deleting…
                </>
              ) : (
                "Delete account"
              )}
            </AlertDialogAction> : null}
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
