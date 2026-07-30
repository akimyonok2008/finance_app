import { useCallback, useMemo, useState } from "react";
import {
  AuthApiError,
  loginWithEmailRequest,
  loginWithGoogleRequest,
  mockLogin,
} from "@/api/authApi";
import { AuthContext } from "@/auth/AuthContext";
import {
  clearStorage,
  readStorage,
  writeStorage,
} from "@/auth/authStorage";
import type {
  AuthContextValue,
  AuthUser,
  LoginFormValues,
} from "@/types/auth";

// import.meta.env.DEV is set by Vite itself based on the build mode (`vite
// build` always produces DEV=false), not by any env var — so this can't be
// flipped on in a production build just by passing a build ARG, unlike
// VITE_ENABLE_MOCK_AUTH alone.
const MOCK_ENABLED =
  import.meta.env.DEV && import.meta.env.VITE_ENABLE_MOCK_AUTH === "true";

export function AuthProvider({ children }: { children: React.ReactNode }) {
  // Initialize synchronously from localStorage — no effect needed.
  const initial = readStorage();
  const [token, setToken] = useState<string | null>(initial.token);
  const [user, setUser] = useState<AuthUser | null>(initial.user);

  const persist = useCallback((t: string, u: AuthUser) => {
    writeStorage(t, u);
    setToken(t);
    setUser(u);
  }, []);

  const login = useCallback(
    async (values: LoginFormValues) => {
      try {
        const session = await loginWithEmailRequest(values);
        persist(session.token, session.user);
      } catch (err) {
        // Only fall back when the backend itself couldn't be reached
        // (AuthApiError means it WAS reached and rejected the request, e.g.
        // wrong password) — otherwise a live backend's real 401 would be
        // silently swallowed and replaced with a fake successful login,
        // masking genuine auth bugs during dev/testing.
        if (MOCK_ENABLED && !(err instanceof AuthApiError)) {
          console.warn(
            "[mock-auth] backend unreachable; signing in as a fake mock user because VITE_ENABLE_MOCK_AUTH=true",
            err,
          );
          const session = await mockLogin(values);
          persist(session.token, session.user);
          return;
        }
        throw err;
      }
    },
    [persist],
  );

  const loginWithGoogle = useCallback(async (credential: string) => {
    const session = await loginWithGoogleRequest(credential);
    persist(session.token, session.user);
  }, [persist]);

  const logout = useCallback(() => {
    void fetch(`${import.meta.env.VITE_API_BASE_URL?.replace(/\/$/, "") || "http://localhost:8080"}/auth/logout`, {
      method: "POST",
      credentials: "include",
    });
    clearStorage();
    setToken(null);
    setUser(null);
  }, []);

  const replaceToken = useCallback((nextToken: string, userUpdates: Partial<AuthUser> = {}) => {
    if (!user) return;
    persist(nextToken, { ...user, ...userUpdates });
  }, [persist, user]);

  const acceptSession = useCallback((session: { token: string; user: AuthUser }) => {
    persist(session.token, session.user);
  }, [persist]);

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      token,
      isAuthenticated: !!token && !!user,
      isBootstrapping: false,
      login,
      loginWithGoogle,
      logout,
      replaceToken,
      acceptSession,
    }),
    [user, token, login, loginWithGoogle, logout, replaceToken, acceptSession],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
