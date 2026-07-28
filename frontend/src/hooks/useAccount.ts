import { useMutation } from "@tanstack/react-query";
import { toast } from "sonner";

import {
  changePasswordRequest,
  deleteAccountRequest,
  reauthenticateWithPasswordRequest,
  setFirstPasswordRequest,
} from "@/api/accountApi";
import { useAuth } from "@/auth/useAuth";

export function useChangePassword() {
  const { replaceToken } = useAuth();
  return useMutation({
    mutationFn: ({
      currentPassword,
      newPassword,
    }: {
      currentPassword: string;
      newPassword: string;
    }) => changePasswordRequest(currentPassword, newPassword),
    onSuccess: ({ token }) => {
      replaceToken(token);
      toast.success("Password updated");
    },
  });
}

// useDeleteAccount deliberately has no onSuccess side effect: the caller
// (AccountSettingsPage) must clear the local session and navigate away itself,
// since that needs useAuth()/useNavigate() and depends on the surrounding page.
export function useDeleteAccount() {
  return useMutation({
    mutationFn: async (password: string) => {
      const reauth = await reauthenticateWithPasswordRequest(password);
      return deleteAccountRequest(reauth.reauthentication_token);
    },
  });
}

export function useSetFirstPassword() {
  const { replaceToken } = useAuth();
  return useMutation({
    mutationFn: ({
      reauthenticationToken,
      newPassword,
    }: {
      reauthenticationToken: string;
      newPassword: string;
    }) => setFirstPasswordRequest(reauthenticationToken, newPassword),
    onSuccess: ({ token }) => {
      replaceToken(token, { has_password: true });
      toast.success("Password created");
    },
  });
}
