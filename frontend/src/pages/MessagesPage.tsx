import { Navigate, useParams, useSearchParams } from "react-router-dom";

export function MessagesPage() {
  const { conversationId } = useParams();
  const [searchParams] = useSearchParams();
  const legacyConversation = searchParams.get("conversation");
  const targetConversation = conversationId || legacyConversation;
  const to = targetConversation
    ? `/explore?tab=messages&conversationId=${encodeURIComponent(targetConversation)}`
    : "/explore?tab=messages";

  return <Navigate to={to} replace />;
}
