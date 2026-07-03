import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import {
  createConversation,
  getConversations,
  getMessages,
  sendMessage,
} from "@/api/dm";
import { queryKeys } from "@/hooks/queryKeys";

export function useConversations() {
  return useQuery({
    queryKey: queryKeys.dmConversations,
    queryFn: ({ signal }) => getConversations(signal),
    refetchInterval: 30_000,
  });
}

export function useMessages(conversationId: string | null) {
  return useQuery({
    queryKey: queryKeys.dmMessages(conversationId ?? ""),
    queryFn: ({ signal }) => getMessages(conversationId ?? "", signal),
    enabled: Boolean(conversationId),
    refetchInterval: conversationId ? 30_000 : false,
  });
}

export function useCreateConversation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (handle: string) => createConversation(handle),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.dmConversations });
    },
    onError: (error: Error) => toast.error(error.message),
  });
}

export function useSendMessage(conversationId: string | null) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: string) => {
      if (!conversationId) throw new Error("Select a conversation first.");
      return sendMessage(conversationId, body);
    },
    onSuccess: () => {
      if (conversationId) {
        queryClient.invalidateQueries({ queryKey: queryKeys.dmMessages(conversationId) });
      }
      queryClient.invalidateQueries({ queryKey: queryKeys.dmConversations });
    },
    onError: (error: Error) => toast.error(error.message),
  });
}
