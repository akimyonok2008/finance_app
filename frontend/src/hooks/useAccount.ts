import { useMutation } from "@tanstack/react-query";
import { toast } from "sonner";

import { changePasswordRequest, deleteAccountRequest } from "@/api/accountApi";

export function useChangePassword() {
  return useMutation({
    mutationFn: ({
      currentPassword,
      newPassword,
    }: {
      currentPassword: string;
      newPassword: string;
    }) => changePasswordRequest(currentPassword, newPassword),
    onSuccess: () => {
      toast.success("Password updated");
    },
  });
}

// useDeleteAccount deliberately has no onSuccess side effect: the caller
// (AccountSettingsPage) must clear the local session and navigate away itself,
// since that needs useAuth()/useNavigate() and depends on the surrounding page.
export function useDeleteAccount() {
  return useMutation({
    mutationFn: (password: string) => deleteAccountRequest(password),
  });
}
