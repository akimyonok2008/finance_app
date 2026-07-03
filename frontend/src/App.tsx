import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { Toaster } from "sonner";

import { AuthProvider } from "@/auth/AuthProvider";
import { ProtectedRoute } from "@/auth/ProtectedRoute";
import { DashboardPage } from "@/pages/Dashboard/DashboardPage";
import { LoginPage } from "@/pages/auth/LoginPage";
import { RegisterPage } from "@/pages/auth/RegisterPage";
import { PortfolioPage } from "@/pages/PortfolioPage";
import { LeaderboardPage } from "@/pages/leaderboard/LeaderboardPage";
import { MyProfilePage } from "@/pages/Profile/MyProfilePage";
import { PublicProfilePage } from "@/pages/Profile/PublicProfilePage";
import { ExplorePage } from "@/pages/Explore/ExplorePage";
import { FriendsPage } from "@/pages/FriendsPage";
import { MessagesPage } from "@/pages/MessagesPage";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 30_000,
    },
  },
});

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <QueryClientProvider client={queryClient}>
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
              path="/coach"
              element={<Navigate to="/portfolio?tab=coach" replace />}
            />

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
              element={<Navigate to="/leaderboard" replace />}
            />

            {/* Redirect root to the overview hub. */}
            <Route path="/" element={<Navigate to="/dashboard" replace />} />

            {/* Fallback for unimplemented routes */}
            <Route path="*" element={<Navigate to="/dashboard" replace />} />
          </Routes>

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
