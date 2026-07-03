import { apiRequest } from "@/api/client";
import type {
  ConversationResponse,
  ConversationsResponse,
  MessageResponse,
  MessagesResponse,
} from "@/types/dm";

export function getConversations(signal?: AbortSignal): Promise<ConversationsResponse> {
  return apiRequest<ConversationsResponse>("/dm/conversations", { signal });
}

export function createConversation(handle: string): Promise<ConversationResponse> {
  return apiRequest<ConversationResponse>("/dm/conversations", {
    method: "POST",
    body: { handle },
  });
}

export function getMessages(
  conversationId: string,
  signal?: AbortSignal,
): Promise<MessagesResponse> {
  return apiRequest<MessagesResponse>(
    `/dm/conversations/${encodeURIComponent(conversationId)}/messages`,
    { signal },
  );
}

export function sendMessage(
  conversationId: string,
  body: string,
): Promise<MessageResponse> {
  return apiRequest<MessageResponse>(
    `/dm/conversations/${encodeURIComponent(conversationId)}/messages`,
    {
      method: "POST",
      body: { body },
    },
  );
}
