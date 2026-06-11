import type { AuthSession, LoginFormValues } from "@/types/auth";

const API_BASE_URL =
  import.meta.env.VITE_API_BASE_URL?.replace(/\/$/, "") ||
  "http://localhost:8080";

const AUTH_LOGIN_PATH =
  import.meta.env.VITE_AUTH_LOGIN_PATH || "/auth/login";

const AUTH_REGISTER_PATH =
  import.meta.env.VITE_AUTH_REGISTER_PATH || "/auth/register";

export type RegisterInput = {
  email: string;
  password: string;
  display_name: string;
};

function normalizeUser(data: Record<string, unknown>): AuthSession["user"] {
  return {
    id: String(data.id ?? ""),
    email: String(data.email ?? ""),
    display_name: String(data.display_name ?? data.displayName ?? ""),
    avatar_key: data.avatar_key ? String(data.avatar_key) : undefined,
  };
}

export async function loginWithEmailRequest(
  values: LoginFormValues,
): Promise<AuthSession> {
  const res = await fetch(`${API_BASE_URL}${AUTH_LOGIN_PATH}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ email: values.email, password: values.password }),
  });

  const data = await res.json().catch(() => ({}));

  if (!res.ok) {
    throw new Error(
      (data as { error?: string }).error ||
        `Sign in failed (${res.status})`,
    );
  }

  const payload = data as Record<string, unknown>;
  const token = String(payload.token ?? "");
  const userRaw =
    (payload.user as Record<string, unknown>) ?? payload;

  return { token, user: normalizeUser(userRaw) };
}

export async function registerRequest(
  input: RegisterInput,
): Promise<AuthSession> {
  const res = await fetch(`${API_BASE_URL}${AUTH_REGISTER_PATH}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });

  const data = await res.json().catch(() => ({}));

  if (!res.ok) {
    throw new Error(
      (data as { error?: string }).error ||
        `Registration failed (${res.status})`,
    );
  }

  const payload = data as Record<string, unknown>;
  const token = String(payload.token ?? "");
  const userRaw =
    (payload.user as Record<string, unknown>) ?? payload;

  return { token, user: normalizeUser(userRaw) };
}

export async function mockLogin(values: LoginFormValues): Promise<AuthSession> {
  await new Promise((r) =>
    setTimeout(r, 700 + Math.random() * 200),
  );
  return {
    token: "mock-jwt-prototype-token",
    user: {
      id: "mock-user-id",
      email: values.email,
      display_name: "AlphaWolf_91",
      avatar_key: "fox",
    },
  };
}
