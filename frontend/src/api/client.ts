/**
 * Thin fetch wrapper for the Go backend.
 *
 * Authentication stores the backend JWT in localStorage under
 * `TOKEN_STORAGE_KEY`, and every authenticated request attaches it here.
 */

export const TOKEN_STORAGE_KEY = "finance_app_token";

const BASE_URL =
  import.meta.env.VITE_API_BASE_URL?.replace(/\/$/, "") ||
  "http://localhost:8080";

/** Error carrying the backend `{ error }` message and HTTP status. */
export class ApiError extends Error {
  status: number;
  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export function getToken(): string | null {
  try {
    return localStorage.getItem(TOKEN_STORAGE_KEY);
  } catch {
    return null;
  }
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_STORAGE_KEY, token);
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_STORAGE_KEY);
}

function handleUnauthorized(): void {
  clearToken();
  localStorage.removeItem("finance_app_user");
  // Hard-redirect to login so the router picks it up cleanly.
  if (typeof window !== "undefined" && !window.location.pathname.startsWith("/login")) {
    window.location.href = "/login";
  }
}

type RequestOptions = {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  body?: unknown;
  signal?: AbortSignal;
};

/**
 * Perform a JSON request, attaching the JWT and normalizing backend errors into
 * an {@link ApiError} that always carries a human-readable message.
 */
export async function apiRequest<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const { method = "GET", body, signal } = options;

  const headers: Record<string, string> = { Accept: "application/json" };
  const token = getToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;
  if (body !== undefined) headers["Content-Type"] = "application/json";

  let res: Response;
  try {
    res = await fetch(`${BASE_URL}${path}`, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
      signal,
    });
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") throw err;
    throw new ApiError(
      "Cannot reach the server. Check that the backend is running.",
      0,
    );
  }

  if (res.status === 401) {
    handleUnauthorized();
    throw new ApiError("Your session has expired. Please sign in again.", 401);
  }

  // 204 / empty body (e.g. DELETE) — nothing to parse.
  if (res.status === 204) {
    return undefined as T;
  }

  const text = await res.text();
  let data: unknown = undefined;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = undefined;
    }
  }

  if (!res.ok) {
    const message =
      (data && typeof data === "object" && "error" in data
        ? String((data as { error: unknown }).error)
        : undefined) || `Request failed (${res.status})`;
    throw new ApiError(message, res.status);
  }

  return data as T;
}
