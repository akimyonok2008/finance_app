import type { AuthUser } from "@/types/auth";

export const TOKEN_KEY = "finance_app_token";
export const USER_KEY = "finance_app_user";

export function readStorage(): { token: string | null; user: AuthUser | null } {
  try {
    const token = localStorage.getItem(TOKEN_KEY);
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
        }
      : null;
    return { token, user };
  } catch {
    return { token: null, user: null };
  }
}

export function writeStorage(token: string, user: AuthUser) {
  localStorage.setItem(TOKEN_KEY, token);
  localStorage.setItem(USER_KEY, JSON.stringify(user));
}

export function clearStorage() {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(USER_KEY);
}
