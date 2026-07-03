import { Navigate } from "react-router-dom";

export function FriendsPage() {
  return <Navigate to="/explore?tab=friends" replace />;
}
