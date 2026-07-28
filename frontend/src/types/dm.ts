import type { SafeProfile } from "@/types/social";

export type LastMessagePreview = {
  body_preview: string;
  sent_at: string;
  sent_by_me: boolean;
};

export type ConversationSummary = {
  id: string;
  other_user: SafeProfile;
  last_message?: LastMessagePreview;
  updated_at: string;
  unread_count: number;
};

export type ConversationsResponse = {
  conversations: ConversationSummary[];
};

export type ConversationResponse = {
  conversation: ConversationSummary;
};

export type Message = {
  id: string;
  conversation_id: string;
  sender: SafeProfile;
  body: string;
  removed: boolean;
  sent_by_me: boolean;
  created_at: string;
};

export type MessagesResponse = {
  messages: Message[];
};

export type MessageResponse = {
  message: Message;
};

export type UnreadCountResponse = {
  count: number;
};
