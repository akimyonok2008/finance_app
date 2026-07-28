export type AuthUser = {
  id: string;
  email: string;
  display_name: string;
  avatar_key?: string;
  email_verified: boolean;
  has_password: boolean;
  role?: "user" | "moderator" | "admin";
  suspended_until?: string;
  suspension_reason?: string;
};

export type AuthSession = {
  token: string;
  user: AuthUser;
};

export type LoginFormValues = {
  email: string;
  password: string;
};

export type AuthContextValue = {
  user: AuthUser | null;
  token: string | null;
  isAuthenticated: boolean;
  isBootstrapping: boolean;
  login: (values: LoginFormValues) => Promise<void>;
  loginWithGoogle: (credential: string) => Promise<void>;
  logout: () => void;
  replaceToken: (token: string, userUpdates?: Partial<AuthUser>) => void;
  acceptSession: (session: AuthSession) => void;
};
