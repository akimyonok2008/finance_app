import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";

import {
  blockUser,
  getBlockedUsers,
  getNotifications,
  getReport,
  getUnreadNotificationCount,
  listReports,
  markAllNotificationsRead,
  markNotificationRead,
  reportMessage,
  reportUser,
  resolveReport,
  unblockUser,
} from "@/api/safety";
import { getUnreadCount, hideMessage, markConversationRead } from "@/api/dm";
import { queryKeys } from "@/hooks/queryKeys";
import type { ReportCategory, ResolveReportInput } from "@/types/safety";

export function useBlockedUsers() {
  return useQuery({
    queryKey: queryKeys.blockedUsers,
    queryFn: ({ signal }) => getBlockedUsers(signal),
  });
}

export function useBlockUser() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (handle: string) => blockUser(handle),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.blockedUsers });
      queryClient.invalidateQueries({ queryKey: queryKeys.friends });
      queryClient.invalidateQueries({ queryKey: queryKeys.following });
      queryClient.invalidateQueries({ queryKey: queryKeys.followers });
      queryClient.invalidateQueries({ queryKey: queryKeys.dmConversations });
      toast.success("User blocked.");
    },
    onError: (error: Error) => toast.error(error.message),
  });
}

export function useUnblockUser() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (handle: string) => unblockUser(handle),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.blockedUsers });
      toast.success("User unblocked.");
    },
    onError: (error: Error) => toast.error(error.message),
  });
}

export function useReportUser() {
  return useMutation({
    mutationFn: ({
      userId,
      category,
      explanation,
    }: {
      userId: string;
      category: ReportCategory;
      explanation: string;
    }) => reportUser(userId, category, explanation),
    onSuccess: () => toast.success("Report submitted. Our moderators will review it."),
    onError: (error: Error) => toast.error(error.message),
  });
}

export function useReportMessage() {
  return useMutation({
    mutationFn: ({
      messageId,
      category,
      explanation,
    }: {
      messageId: string;
      category: ReportCategory;
      explanation: string;
    }) => reportMessage(messageId, category, explanation),
    onSuccess: () => toast.success("Report submitted. Our moderators will review it."),
    onError: (error: Error) => toast.error(error.message),
  });
}

export function useHideMessage(conversationId: string | null) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (messageId: string) => hideMessage(messageId),
    onSuccess: () => {
      if (conversationId) {
        queryClient.invalidateQueries({ queryKey: queryKeys.dmMessages(conversationId) });
      }
      queryClient.invalidateQueries({ queryKey: queryKeys.dmConversations });
    },
    onError: (error: Error) => toast.error(error.message),
  });
}

export function useMarkConversationRead(conversationId: string | null) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (lastReadMessageId: string) => {
      if (!conversationId) throw new Error("No conversation selected.");
      return markConversationRead(conversationId, lastReadMessageId);
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.dmConversations });
      queryClient.invalidateQueries({ queryKey: queryKeys.dmUnreadCount });
    },
  });
}

export function useDMUnreadCount() {
  return useQuery({
    queryKey: queryKeys.dmUnreadCount,
    queryFn: ({ signal }) => getUnreadCount(signal),
    refetchInterval: 30_000,
  });
}

export function useNotifications() {
  return useQuery({
    queryKey: queryKeys.notifications,
    queryFn: ({ signal }) => getNotifications(signal),
    refetchInterval: 30_000,
  });
}

export function useNotificationsUnreadCount() {
  return useQuery({
    queryKey: queryKeys.notificationsUnreadCount,
    queryFn: ({ signal }) => getUnreadNotificationCount(signal),
    refetchInterval: 30_000,
  });
}

export function useMarkNotificationRead() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (notificationId: string) => markNotificationRead(notificationId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.notifications });
      queryClient.invalidateQueries({ queryKey: queryKeys.notificationsUnreadCount });
    },
  });
}

export function useMarkAllNotificationsRead() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => markAllNotificationsRead(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: queryKeys.notifications });
      queryClient.invalidateQueries({ queryKey: queryKeys.notificationsUnreadCount });
    },
  });
}

// --- moderator/admin only ---------------------------------------------------

export function useModerationReports(status?: string) {
  return useQuery({
    queryKey: queryKeys.moderationReports(status),
    queryFn: ({ signal }) => listReports(status, signal),
  });
}

export function useModerationReport(reportId: string | null) {
  return useQuery({
    queryKey: queryKeys.moderationReport(reportId ?? ""),
    queryFn: ({ signal }) => getReport(reportId ?? "", signal),
    enabled: Boolean(reportId),
  });
}

export function useResolveReport() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ reportId, input }: { reportId: string; input: ResolveReportInput }) =>
      resolveReport(reportId, input),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: queryKeys.moderationReports() });
      queryClient.invalidateQueries({ queryKey: queryKeys.moderationReport(variables.reportId) });
      toast.success("Report resolved.");
    },
    onError: (error: Error) => toast.error(error.message),
  });
}
