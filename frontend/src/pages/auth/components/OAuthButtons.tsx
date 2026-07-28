import { useCallback, useEffect, useRef, useState } from "react";

import { useAuth } from "@/auth/useAuth";

declare global {
  interface Window {
    google?: {
      accounts: {
        id: {
          initialize: (config: {
            client_id: string;
            callback: (response: { credential?: string }) => void;
            ux_mode?: "popup" | "redirect";
            auto_select?: boolean;
            cancel_on_tap_outside?: boolean;
          }) => void;
          renderButton: (
            parent: HTMLElement,
            options: {
              theme?: "outline" | "filled_blue" | "filled_black";
              size?: "large" | "medium" | "small";
              type?: "standard" | "icon";
              text?: "signin_with" | "signup_with" | "continue_with" | "signin";
              shape?: "rectangular" | "pill" | "circle" | "square";
              logo_alignment?: "left" | "center";
              width?: number;
            },
          ) => void;
        };
      };
    };
  }
}

const GOOGLE_ENABLED = import.meta.env.VITE_GOOGLE_AUTH_ENABLED === "true";
const GOOGLE_CLIENT_ID = import.meta.env.VITE_GOOGLE_CLIENT_ID || "";
const scriptPromises = new Map<string, Promise<void>>();

type Props = {
  disabled?: boolean;
  onSuccess: () => void;
  onError: (message: string) => void;
};

export function OAuthButtons({ disabled, onSuccess, onError }: Props) {
  const { loginWithGoogle } = useAuth();
  const [loading, setLoading] = useState(false);

  if (!GOOGLE_ENABLED) return null;

  return (
    <div className="space-y-2">
      <GoogleSignInButton
        loading={loading}
        disabled={disabled || loading}
        onStart={() => setLoading(true)}
        onDone={() => setLoading(false)}
        onSuccess={onSuccess}
        onError={onError}
        loginWithGoogle={loginWithGoogle}
      />
    </div>
  );
}

export function GoogleSignInButton({
  loading,
  disabled,
  onStart,
  onDone,
  onSuccess,
  onError,
  loginWithGoogle,
}: {
  loading?: boolean;
  disabled?: boolean;
  onStart: () => void;
  onDone: () => void;
  onSuccess: () => void;
  onError: (message: string) => void;
  loginWithGoogle: (credential: string) => Promise<void>;
}) {
  const buttonRef = useRef<HTMLDivElement | null>(null);
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading");

  const handleCredential = useCallback(
    async (credential?: string) => {
      if (!credential) {
        onError("Google sign-in failed.");
        return;
      }
      onStart();
      try {
        await loginWithGoogle(credential);
        onSuccess();
      } catch {
        onError("Google sign-in failed.");
      } finally {
        onDone();
      }
    },
    [loginWithGoogle, onDone, onError, onStart, onSuccess],
  );

  const initializeGoogle = useCallback(async () => {
    if (!GOOGLE_CLIENT_ID) {
      onError("This sign-in method is not configured.");
      return false;
    }
    try {
      await loadScript("google-identity-services", "https://accounts.google.com/gsi/client");
      if (!window.google?.accounts.id) return false;
      window.google.accounts.id.initialize({
        client_id: GOOGLE_CLIENT_ID,
        callback: (response) => void handleCredential(response.credential),
        ux_mode: "popup",
        auto_select: false,
        cancel_on_tap_outside: true,
      });
      return true;
    } catch {
      return false;
    }
  }, [handleCredential, onError]);

  useEffect(() => {
    let cancelled = false;

    const renderGoogleButton = async () => {
      setStatus("loading");
      const ready = await initializeGoogle();
      if (cancelled || !ready || !buttonRef.current || !window.google?.accounts.id) {
        if (!cancelled) setStatus("error");
        return;
      }

      for (let attempt = 0; attempt < 10; attempt += 1) {
        if (cancelled || !buttonRef.current) return;
        const width = Math.floor(
          Math.min(336, buttonRef.current.getBoundingClientRect().width || 336),
        );
        buttonRef.current.innerHTML = "";
        window.google.accounts.id.renderButton(buttonRef.current, {
          type: "standard",
          theme: "outline",
          size: "large",
          text: "continue_with",
          shape: "rectangular",
          logo_alignment: "left",
          width,
        });
        if (buttonRef.current.childElementCount > 0) {
          setStatus("ready");
          return;
        }
        await new Promise((resolve) => window.setTimeout(resolve, 150));
      }
      if (!cancelled) setStatus("error");
    };

    void renderGoogleButton();

    return () => {
      cancelled = true;
    };
  }, [initializeGoogle]);

  return (
    <div className="min-h-11">
      <div
        ref={buttonRef}
        aria-hidden={disabled || loading || status !== "ready" ? "true" : undefined}
        className={`w-full ${disabled || loading || status !== "ready" ? "pointer-events-none opacity-0" : ""}`}
      />
      {status === "loading" && (
        <div className="h-11 w-full animate-pulse rounded-xl border border-zinc-800 bg-zinc-900/70" />
      )}
      {status === "error" && (
        <button
          type="button"
          disabled
          className="flex h-11 w-full items-center justify-center rounded-xl border border-zinc-800 bg-zinc-900/60 text-sm font-medium text-zinc-500"
        >
          Google sign-in unavailable
        </button>
      )}
    </div>
  );
}

function loadScript(id: string, src: string): Promise<void> {
  const existing = document.getElementById(id) as HTMLScriptElement | null;
  if (existing?.dataset.loaded === "true") return Promise.resolve();

  const pending = scriptPromises.get(id);
  if (pending) return pending;

  const promise = new Promise<void>((resolve, reject) => {
    const script = existing ?? document.createElement("script");

    const cleanup = () => {
      script.removeEventListener("load", handleLoad);
      script.removeEventListener("error", handleError);
    };
    const handleLoad = () => {
      script.dataset.loaded = "true";
      cleanup();
      resolve();
    };
    const handleError = () => {
      scriptPromises.delete(id);
      cleanup();
      reject(new Error("script failed"));
    };

    script.id = id;
    script.src = src;
    script.async = true;
    script.defer = true;
    script.addEventListener("load", handleLoad);
    script.addEventListener("error", handleError);
    if (!existing) document.head.appendChild(script);
  });

  scriptPromises.set(id, promise);
  return promise;
}
