import { useCallback, useMemo, useState } from "react";
import {
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

const MOCK_ENABLED = import.meta.env.VITE_ENABLE_MOCK_AUTH === "true";

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
        if (MOCK_ENABLED) {
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
    clearStorage();
    setToken(null);
    setUser(null);
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({
      user,
      token,
      isAuthenticated: !!token && !!user,
      isBootstrapping: false,
      login,
      loginWithGoogle,
      logout,
    }),
    [user, token, login, loginWithGoogle, logout],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}
