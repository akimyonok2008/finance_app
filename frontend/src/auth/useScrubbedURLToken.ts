import { useLayoutEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";

/**
 * Captures a one-time token for the current render, then removes it from the
 * visible browser URL before passive effects or API requests can run.
 */
export function useScrubbedURLToken(): string {
  const [params] = useSearchParams();
  const [token] = useState(() => params.get("token") ?? "");

  useLayoutEffect(() => {
    const url = new URL(window.location.href);
    if (!url.searchParams.has("token")) return;
    url.searchParams.delete("token");
    window.history.replaceState(window.history.state, "", `${url.pathname}${url.search}${url.hash}`);
  }, []);

  return token;
}
