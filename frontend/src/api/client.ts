/**
 * Thin fetch wrapper for the Go backend.
 *
 * Browser authentication uses an HttpOnly session cookie. JavaScript never
 * reads or persists the JWT.
 */

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

function handleUnauthorized(): void {
  // Remove values left by versions that predated HttpOnly cookie sessions.
  localStorage.removeItem("finance_app_token");
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
  idempotencyKey?: string;
  /**
   * Some authenticated endpoints legitimately return 401 for a reason OTHER
   * than an invalid/expired token — e.g. change-password and delete-account
   * re-verify the current password and return 401 for a WRONG password. That
   * is not a session problem, so treating it as one would wrongly log the
   * caller out and discard their form input. Pass true to let a 401 surface
   * as a normal ApiError instead of triggering the global session-expired
   * redirect.
   */
  skipAuthRedirectOn401?: boolean;
};

/**
 * Perform a JSON request, attaching the JWT and normalizing backend errors into
 * an {@link ApiError} that always carries a human-readable message.
 */
export async function apiRequest<T>(
  path: string,
  options: RequestOptions = {},
): Promise<T> {
  const { method = "GET", body, signal, idempotencyKey, skipAuthRedirectOn401 } = options;

  const headers: Record<string, string> = { Accept: "application/json" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (idempotencyKey) headers["Idempotency-Key"] = idempotencyKey;

  let res: Response;
  try {
    res = await fetch(`${BASE_URL}${path}`, {
      method,
      headers,
      credentials: "include",
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

  if (res.status === 401 && !skipAuthRedirectOn401) {
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
