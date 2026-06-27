import { CircleUserRound, Compass, LayoutDashboard, LogOut, Medal, Sparkles, WalletCards } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { Link, useLocation, useNavigate } from "react-router-dom";

import { useAuth } from "@/auth/useAuth";
import { cn } from "@/utils/cn";

type NavItem = {
  to: string;
  label: string;
  icon: LucideIcon;
};

const NAV_ITEMS: NavItem[] = [
  { to: "/dashboard", label: "Dashboard", icon: LayoutDashboard },
  { to: "/portfolio", label: "Portfolio", icon: WalletCards },
  { to: "/leaderboard", label: "Leaderboard", icon: Medal },
  { to: "/explore", label: "Explore", icon: Compass },
  { to: "/profile", label: "Profile", icon: CircleUserRound },
  { to: "/coach", label: "Coach", icon: Sparkles },
];

type AppNavProps = {
  /** Optional extra actions (e.g. a Refresh button) rendered before Sign out. */
  actions?: React.ReactNode;
};

/**
 * Persistent top navigation shared across all authenticated screens so users can
 * always move between the main product screens.
 */
export function AppNav({ actions }: AppNavProps) {
  const location = useLocation();
  const navigate = useNavigate();
  const { logout } = useAuth();

  const handleLogout = () => {
    logout();
    navigate("/login");
  };

  return (
    <nav className="mb-8 flex items-center justify-between gap-2 rounded-xl border border-zinc-800 bg-zinc-900/40 p-1">
      <div className="flex items-center gap-1 overflow-x-auto">
        {NAV_ITEMS.map(({ to, label, icon: Icon }) => {
          const active = location.pathname.startsWith(to);
          return (
            <Link
              key={to}
              to={to}
              aria-current={active ? "page" : undefined}
              className={cn(
                "flex shrink-0 items-center gap-2 rounded-lg px-3 py-2 text-xs font-medium transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500",
                active
                  ? "bg-zinc-50 text-zinc-950"
                  : "text-zinc-400 hover:bg-zinc-800/70 hover:text-zinc-100",
              )}
            >
              <Icon className="h-3.5 w-3.5" />
              {label}
            </Link>
          );
        })}
      </div>

      <div className="flex items-center gap-1">
        {actions}
        <button
          type="button"
          onClick={handleLogout}
          aria-label="Sign out"
          className="rounded-lg p-2 text-zinc-400 transition hover:bg-zinc-800/70 hover:text-zinc-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-zinc-500"
        >
          <LogOut className="h-3.5 w-3.5" />
        </button>
      </div>
    </nav>
  );
}
