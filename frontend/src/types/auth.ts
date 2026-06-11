export type AuthUser = {
  id: string;
  email: string;
  display_name: string;
  avatar_key?: string;
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
  loginWithGoogle: () => Promise<void>;
  logout: () => void;
};
