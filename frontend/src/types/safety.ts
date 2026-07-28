export type BlockStateResponse = {
  handle: string;
  is_blocked: boolean;
};

export type BlockedUserItem = {
  handle: string;
  display_name: string;
  avatar_key?: string;
  blocked_at: string;
};

export type BlockedUsersResponse = {
  blocked_users: BlockedUserItem[];
};

export const REPORT_CATEGORIES = [
  "harassment",
  "spam",
  "impersonation",
  "fraud_or_scam",
  "hate_or_abuse",
  "sexual_content",
  "threats",
  "privacy_violation",
  "other",
] as const;

export type ReportCategory = (typeof REPORT_CATEGORIES)[number];

export type ReportResponse = {
  report_id: string;
  status: string;
};

export type NotificationDTO = {
  id: string;
  type: string;
  payload: Record<string, string>;
  read: boolean;
  created_at: string;
};

export type NotificationsResponse = {
  notifications: NotificationDTO[];
};

export type UnreadCountResponse = {
  count: number;
};

export type ReportSummary = {
  id: string;
  reporter_user_id: string;
  reported_user_id: string;
  message_id?: string;
  category: string;
  status: string;
  created_at: string;
};

export type ReportListResponse = {
  reports: ReportSummary[];
};

export type EvidenceDTO = {
  message_text?: string;
  message_created_at?: string;
  conversation_id?: string;
  sender_id?: string;
};

export type ReportDetail = ReportSummary & {
  explanation: string;
  reviewed_at?: string;
  reviewer_id?: string;
  decision?: string;
  moderator_notes?: string;
  evidence?: EvidenceDTO;
};

export type ResolveReportInput = {
  decision: "action_taken" | "no_action";
  action_type?:
    | "no_action"
    | "warning"
    | "temporary_suspension"
    | "permanent_ban"
    | "content_removal";
  reason?: string;
  moderator_notes?: string;
  suspension_days?: number;
};
