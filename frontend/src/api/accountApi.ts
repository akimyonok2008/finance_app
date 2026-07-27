import { apiRequest } from "@/api/client";

/**
 * Authenticated account-lifecycle actions. These require the current
 * password even though the caller already holds a valid JWT: a sensitive or
 * destructive account action re-confirms the credential rather than trusting
 * session possession alone.
 */

export function changePasswordRequest(
  currentPassword: string,
  newPassword: string,
): Promise<void> {
  return apiRequest<void>("/auth/change-password", {
    method: "POST",
    body: { current_password: currentPassword, new_password: newPassword },
    // A wrong current password is a 401 that means "wrong password", not
    // "your session expired" — don't log the user out over it.
    skipAuthRedirectOn401: true,
  });
}

export function deleteAccountRequest(password: string): Promise<void> {
  return apiRequest<void>("/auth/delete-account", {
    method: "POST",
    body: { password },
    // Same reasoning as changePasswordRequest: a wrong password here must not
    // trigger the global session-expired redirect.
    skipAuthRedirectOn401: true,
  });
}
