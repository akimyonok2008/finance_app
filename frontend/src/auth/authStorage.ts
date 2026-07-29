import type { AuthUser } from "@/types/auth";

export const TOKEN_KEY = "finance_app_token";
export const USER_KEY = "finance_app_user";

function accountRole(value: unknown): AuthUser["role"] {
  return value === "user" || value === "moderator" || value === "admin"
    ? value
    : undefined;
}

export function readStorage(): { token: string | null; user: AuthUser | null } {
  try {
    // The JWT lives only in an HttpOnly cookie. This non-secret marker keeps
    // the existing auth context shape while /me validates the cookie.
    localStorage.removeItem(TOKEN_KEY);
    const token = localStorage.getItem(USER_KEY) ? "cookie-session" : null;
    const raw = localStorage.getItem(USER_KEY);
    const parsed = raw ? (JSON.parse(raw) as Partial<AuthUser>) : null;
    const user: AuthUser | null = parsed
      ? {
          id: String(parsed.id ?? ""),
          email: String(parsed.email ?? ""),
          display_name: String(parsed.display_name ?? ""),
          avatar_key: parsed.avatar_key,
          // Existing snapshots predate these safe account-state flags. Their
          // sessions came from password accounts and were already active.
          email_verified: parsed.email_verified ?? true,
          has_password: parsed.has_password ?? true,
          role: accountRole(parsed.role),
          suspended_until: parsed.suspended_until
            ? String(parsed.suspended_until)
            : undefined,
          suspension_reason: parsed.suspension_reason
            ? String(parsed.suspension_reason)
            : undefined,
        }
      : null;
    return { token, user };
  } catch {
    return { token: null, user: null };
  }
}

export function writeStorage(_token: string, user: AuthUser) {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.setItem(USER_KEY, JSON.stringify(user));
}

export function clearStorage() {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(USER_KEY);
}
