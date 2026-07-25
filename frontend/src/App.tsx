import { lazy, Suspense } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { Toaster } from "sonner";

import { AuthProvider } from "@/auth/AuthProvider";
import { ProtectedRoute } from "@/auth/ProtectedRoute";
import { useAuth } from "@/auth/useAuth";

const DashboardPage = lazy(() =>
  import("@/pages/Dashboard/DashboardPage").then(({ DashboardPage }) => ({
    default: DashboardPage,
  })),
);
const LoginPage = lazy(() =>
  import("@/pages/auth/LoginPage").then(({ LoginPage }) => ({
    default: LoginPage,
  })),
);
const RegisterPage = lazy(() =>
  import("@/pages/auth/RegisterPage").then(({ RegisterPage }) => ({
    default: RegisterPage,
  })),
);
const PortfolioPage = lazy(() =>
  import("@/pages/PortfolioPage").then(({ PortfolioPage }) => ({
    default: PortfolioPage,
  })),
);
const LeaderboardPage = lazy(() =>
  import("@/pages/leaderboard/LeaderboardPage").then(({ LeaderboardPage }) => ({
    default: LeaderboardPage,
  })),
);
const MyProfilePage = lazy(() =>
  import("@/pages/Profile/MyProfilePage").then(({ MyProfilePage }) => ({
    default: MyProfilePage,
  })),
);
const PublicProfilePage = lazy(() =>
  import("@/pages/Profile/PublicProfilePage").then(({ PublicProfilePage }) => ({
    default: PublicProfilePage,
  })),
);
const ExplorePage = lazy(() =>
  import("@/pages/Explore/ExplorePage").then(({ ExplorePage }) => ({
    default: ExplorePage,
  })),
);
const FriendsPage = lazy(() =>
  import("@/pages/FriendsPage").then(({ FriendsPage }) => ({
    default: FriendsPage,
  })),
);
const MessagesPage = lazy(() =>
  import("@/pages/MessagesPage").then(({ MessagesPage }) => ({
    default: MessagesPage,
  })),
);
const LandingPage = lazy(() =>
  import("@/pages/LandingPage").then(({ LandingPage }) => ({
    default: LandingPage,
  })),
);
const AchievementsPage = lazy(() =>
  import("@/pages/achievements/AchievementsPage").then(
    ({ AchievementsPage }) => ({
      default: AchievementsPage,
    }),
  ),
);

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 30_000,
    },
  },
});

function PublicHomeRoute() {
  const { isAuthenticated, isBootstrapping } = useAuth();

  if (isBootstrapping) return null;
  if (isAuthenticated) return <Navigate to="/dashboard" replace />;

  return <LandingPage />;
}

function RouteFallback() {
  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50">
      <div className="mx-auto flex min-h-screen max-w-7xl items-center justify-center px-4">
        <div className="h-6 w-6 animate-spin rounded-full border border-zinc-800 border-t-zinc-300" />
      </div>
    </div>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <QueryClientProvider client={queryClient}>
          <Suspense fallback={<RouteFallback />}>
            <Routes>
              <Route path="/login" element={<LoginPage />} />
              <Route path="/register" element={<RegisterPage />} />

              <Route
                path="/dashboard"
                element={
                  <ProtectedRoute>
                    <DashboardPage />
                  </ProtectedRoute>
                }
              />

              <Route
                path="/portfolio"
                element={
                  <ProtectedRoute>
                    <PortfolioPage />
                  </ProtectedRoute>
                }
              />

              {/* Activity is a tab within Portfolio, not a separate page —
                  this redirect keeps old links/bookmarks working. Performance
                  was folded into the Portfolio tab's summary cards. */}
              <Route path="/activity" element={<Navigate to="/portfolio?tab=transactions" replace />} />
              <Route path="/performance" element={<Navigate to="/portfolio" replace />} />

              <Route
                path="/leaderboard"
                element={
                  <ProtectedRoute>
                    <LeaderboardPage />
                  </ProtectedRoute>
                }
              />

              <Route path="/arena" element={<Navigate to="/leaderboard" replace />} />

              <Route
                path="/profile"
                element={
                  <ProtectedRoute>
                    <MyProfilePage />
                  </ProtectedRoute>
                }
              />

              <Route
                path="/profiles/:handle"
                element={
                  <ProtectedRoute>
                    <PublicProfilePage />
                  </ProtectedRoute>
                }
              />

              <Route
                path="/explore"
                element={
                  <ProtectedRoute>
                    <ExplorePage />
                  </ProtectedRoute>
                }
              />

              <Route
                path="/friends"
                element={
                  <ProtectedRoute>
                    <FriendsPage />
                  </ProtectedRoute>
                }
              />

              <Route
                path="/messages"
                element={
                  <ProtectedRoute>
                    <MessagesPage />
                  </ProtectedRoute>
                }
              />

              <Route
                path="/messages/:conversationId"
                element={
                  <ProtectedRoute>
                    <MessagesPage />
                  </ProtectedRoute>
                }
              />

              <Route path="/sprint" element={<Navigate to="/leaderboard" replace />} />
              <Route path="/profile/me" element={<Navigate to="/profile" replace />} />
              <Route
                path="/achievements"
                element={
                  <ProtectedRoute>
                    <AchievementsPage />
                  </ProtectedRoute>
                }
              />

              <Route path="/" element={<PublicHomeRoute />} />

              {/* Fallback for unimplemented routes */}
              <Route path="*" element={<Navigate to="/dashboard" replace />} />
            </Routes>
          </Suspense>

          <Toaster
            theme="dark"
            position="top-right"
            richColors
            closeButton
            toastOptions={{
              style: {
                background: "hsl(222 40% 8%)",
                border: "1px solid hsl(215 28% 17%)",
                color: "hsl(210 40% 98%)",
              },
            }}
          />
        </QueryClientProvider>
      </AuthProvider>
    </BrowserRouter>
  );
}
