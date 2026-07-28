import { apiRequest } from "@/api/client";

type ReplacementToken = { token: string };
export type ReauthenticationResult = { reauthentication_token: string };

export function changePasswordRequest(
  currentPassword: string,
  newPassword: string,
): Promise<ReplacementToken> {
  return apiRequest<ReplacementToken>("/auth/change-password", {
    method: "POST",
    body: { current_password: currentPassword, new_password: newPassword },
    skipAuthRedirectOn401: true,
  });
}

export function setFirstPasswordRequest(
  reauthenticationToken: string,
  newPassword: string,
): Promise<ReplacementToken> {
  return apiRequest<ReplacementToken>("/auth/set-password", {
    method: "POST",
    body: {
      reauthentication_token: reauthenticationToken,
      new_password: newPassword,
    },
    skipAuthRedirectOn401: true,
  });
}

export function reauthenticateWithPasswordRequest(
  password: string,
): Promise<ReauthenticationResult> {
  return apiRequest<ReauthenticationResult>("/auth/reauthenticate", {
    method: "POST",
    body: { password },
    skipAuthRedirectOn401: true,
  });
}

export function reauthenticateWithProviderRequest(
  provider: "google",
  credential: string,
): Promise<ReauthenticationResult> {
  return apiRequest<ReauthenticationResult>("/auth/reauthenticate", {
    method: "POST",
    body: { provider, credential },
    skipAuthRedirectOn401: true,
  });
}

export function deleteAccountRequest(reauthenticationToken: string): Promise<void> {
  return apiRequest<void>("/auth/delete-account", {
    method: "POST",
    body: { reauthentication_token: reauthenticationToken },
    skipAuthRedirectOn401: true,
  });
}

export function revokeSessionsRequest(): Promise<void> {
  return apiRequest<void>("/auth/revoke-sessions", { method: "POST" });
}
