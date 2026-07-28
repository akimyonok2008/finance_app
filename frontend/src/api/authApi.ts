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

export type RegistrationResult = {
  user: AuthSession["user"];
  verification_required: boolean;
};

export class AuthApiError extends Error {
  code?: string;
  status: number;
  constructor(message: string, status: number, code?: string) {
    super(message);
    this.name = "AuthApiError";
    this.status = status;
    this.code = code;
  }
}

function normalizeUser(data: Record<string, unknown>): AuthSession["user"] {
  return {
    id: String(data.id ?? ""),
    email: String(data.email ?? ""),
    display_name: String(data.display_name ?? data.displayName ?? ""),
    avatar_key: data.avatar_key ? String(data.avatar_key) : undefined,
    email_verified: Boolean(data.email_verified),
    has_password: Boolean(data.has_password),
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
    throw new AuthApiError(
      (data as { error?: string }).error ||
        `Sign in failed (${res.status})`,
      res.status,
      (data as { code?: string }).code,
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
): Promise<RegistrationResult> {
  const res = await fetch(`${API_BASE_URL}${AUTH_REGISTER_PATH}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(input),
  });

  const data = await res.json().catch(() => ({}));

  if (!res.ok) {
    throw new AuthApiError(
      (data as { error?: string }).error ||
        `Registration failed (${res.status})`,
      res.status,
      (data as { code?: string }).code,
    );
  }

  const payload = data as Record<string, unknown>;
  const userRaw =
    (payload.user as Record<string, unknown>) ?? payload;

  return {
    user: normalizeUser(userRaw),
    verification_required: Boolean(payload.verification_required),
  };
}

async function publicJSON<T>(path: string, body: unknown): Promise<T> {
  const res = await fetch(`${API_BASE_URL}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new AuthApiError(
      (data as { error?: string }).error || `Request failed (${res.status})`,
      res.status,
      (data as { code?: string }).code,
    );
  }
  return data as T;
}

export function verifyEmailRequest(token: string): Promise<AuthSession> {
  return publicJSON<AuthSession>("/auth/verify-email", { token });
}

export function resendVerificationRequest(email: string): Promise<{ message: string }> {
  return publicJSON("/auth/resend-verification", { email });
}

export function forgotPasswordRequest(email: string): Promise<{ message: string }> {
  return publicJSON("/auth/forgot-password", { email });
}

export async function resetPasswordRequest(token: string, newPassword: string): Promise<void> {
  await publicJSON<unknown>("/auth/reset-password", {
    token,
    new_password: newPassword,
  });
}

export async function loginWithGoogleRequest(
  credential: string,
): Promise<AuthSession> {
  return providerRequest("/auth/google", { credential }, "Google sign-in failed.");
}

async function providerRequest(
  path: string,
  body: unknown,
  fallback: string,
): Promise<AuthSession> {
  const res = await fetch(`${API_BASE_URL}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new AuthApiError(
      (data as { error?: string }).error || fallback,
      res.status,
      (data as { code?: string }).code,
    );
  }
  const payload = data as Record<string, unknown>;
  const token = String(payload.token ?? "");
  const userRaw = (payload.user as Record<string, unknown>) ?? payload;
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
      email_verified: true,
      has_password: true,
    },
  };
}
