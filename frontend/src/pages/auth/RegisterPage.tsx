import { useEffect } from "react";
import { useNavigate } from "react-router-dom";

import { useAuth } from "@/auth/useAuth";
import { AuthBrandPanel } from "@/pages/auth/components/AuthBrandPanel";
import { RegisterCard } from "@/pages/auth/components/RegisterCard";

export function RegisterPage() {
  const { isAuthenticated, isBootstrapping } = useAuth();
  const navigate = useNavigate();

  useEffect(() => {
    if (!isBootstrapping && isAuthenticated) {
      navigate("/dashboard", { replace: true });
    }
  }, [isAuthenticated, isBootstrapping, navigate]);

  if (isBootstrapping) return null;

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50">
      <div className="grid min-h-screen lg:grid-cols-2">
        <AuthBrandPanel />
        <main className="flex min-h-screen items-center justify-center px-4 py-10 sm:px-6 lg:px-8">
          <RegisterCard />
        </main>
      </div>
    </div>
  );
}
