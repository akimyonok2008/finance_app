import { apiRequest } from "@/api/client";
import type {
  BlockedUsersResponse,
  BlockStateResponse,
  NotificationsResponse,
  ReportCategory,
  ReportDetail,
  ReportListResponse,
  ReportResponse,
  ResolveReportInput,
  UnreadCountResponse,
} from "@/types/safety";

export function blockUser(handle: string): Promise<BlockStateResponse> {
  return apiRequest<BlockStateResponse>(`/social/users/${encodeURIComponent(handle)}/block`, {
    method: "POST",
  });
}

export function unblockUser(handle: string): Promise<BlockStateResponse> {
  return apiRequest<BlockStateResponse>(`/social/users/${encodeURIComponent(handle)}/block`, {
    method: "DELETE",
  });
}

export function getBlockedUsers(signal?: AbortSignal): Promise<BlockedUsersResponse> {
  return apiRequest<BlockedUsersResponse>("/me/blocked-users", { signal });
}

export function reportUser(
  userId: string,
  category: ReportCategory,
  explanation: string,
): Promise<ReportResponse> {
  return apiRequest<ReportResponse>(`/reports/users/${encodeURIComponent(userId)}`, {
    method: "POST",
    body: { category, explanation },
  });
}

export function reportMessage(
  messageId: string,
  category: ReportCategory,
  explanation: string,
): Promise<ReportResponse> {
  return apiRequest<ReportResponse>(`/reports/messages/${encodeURIComponent(messageId)}`, {
    method: "POST",
    body: { category, explanation },
  });
}

export function getNotifications(signal?: AbortSignal): Promise<NotificationsResponse> {
  return apiRequest<NotificationsResponse>("/notifications", { signal });
}

export function getUnreadNotificationCount(signal?: AbortSignal): Promise<UnreadCountResponse> {
  return apiRequest<UnreadCountResponse>("/notifications/unread-count", { signal });
}

export function markNotificationRead(notificationId: string): Promise<void> {
  return apiRequest<void>(`/notifications/${encodeURIComponent(notificationId)}/read`, {
    method: "POST",
  });
}

export function markAllNotificationsRead(): Promise<void> {
  return apiRequest<void>("/notifications/read-all", { method: "POST" });
}

// Moderator/admin only.
export function listReports(status?: string, signal?: AbortSignal): Promise<ReportListResponse> {
  const qs = status ? `?status=${encodeURIComponent(status)}` : "";
  return apiRequest<ReportListResponse>(`/moderation/reports${qs}`, { signal });
}

export function getReport(reportId: string, signal?: AbortSignal): Promise<ReportDetail> {
  return apiRequest<ReportDetail>(`/moderation/reports/${encodeURIComponent(reportId)}`, {
    signal,
  });
}

export function resolveReport(
  reportId: string,
  input: ResolveReportInput,
): Promise<ReportDetail> {
  return apiRequest<ReportDetail>(`/moderation/reports/${encodeURIComponent(reportId)}/resolve`, {
    method: "POST",
    body: input,
  });
}
