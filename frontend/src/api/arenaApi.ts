import { apiRequest } from "@/api/client";
import type {
  Achievement,
  ArenaCompetitionCard,
  CompetitionStatus,
  EligibilityPreview,
  JoinResult,
  LeaderboardPage,
  LeaderboardRow,
} from "@/types/arena";
import type { Achievement as LegacyAchievement } from "@/types/dashboard";

// --- wire shapes (snake_case, decimal-strings) --------------------------------

type ArenaCardWire = {
  id: string;
  name: string;
  description?: string;
  category?: string;
  icon_key?: string;
  status: string;
  starts_at: string;
  ends_at: string;
  join_opens_at?: string;
  join_closes_at?: string;
  scoring_scope?: string;
  participant_count: number;
  joined: boolean;
  entry_status?: string;
};

function fromCardWire(w: ArenaCardWire): ArenaCompetitionCard {
  return {
    id: w.id,
    name: w.name,
    description: w.description,
    category: w.category,
    iconKey: w.icon_key,
    status: w.status,
    startsAt: w.starts_at,
    endsAt: w.ends_at,
    joinOpensAt: w.join_opens_at,
    joinClosesAt: w.join_closes_at,
    scoringScope: w.scoring_scope,
    participantCount: w.participant_count,
    joined: w.joined,
    entryStatus: w.entry_status,
  };
}

export async function getArenaCatalogue(
  filters: { category?: string; joined?: boolean } = {},
  signal?: AbortSignal,
): Promise<ArenaCompetitionCard[]> {
  const params = new URLSearchParams();
  if (filters.category) params.set("category", filters.category);
  if (filters.joined !== undefined) params.set("joined", String(filters.joined));
  const query = params.toString();
  const cards = await apiRequest<ArenaCardWire[]>(
    `/arena/competitions${query ? `?${query}` : ""}`,
    { signal },
  );
  return cards.map(fromCardWire);
}

export async function getArenaCatalogueItem(
  competitionId: string,
  signal?: AbortSignal,
): Promise<ArenaCompetitionCard> {
  const card = await apiRequest<ArenaCardWire>(
    `/arena/competitions/${competitionId}`,
    { signal },
  );
  return fromCardWire(card);
}

type EligibilityWire = {
  eligible: boolean;
  evaluated_at: string;
  rules: {
    code: string;
    label: string;
    required: string;
    actual: string;
    passed: boolean;
    reason?: string;
  }[];
};

export async function getEligibility(
  competitionId: string,
  signal?: AbortSignal,
): Promise<EligibilityPreview> {
  const w = await apiRequest<EligibilityWire>(
    `/competitions/${competitionId}/eligibility`,
    { signal },
  );
  return { eligible: w.eligible, evaluatedAt: w.evaluated_at, rules: w.rules };
}

type MyStatusWire = {
  competition_id: string;
  joined: boolean;
  entry_status?: string;
  current_rank: number;
  sprint_return_percentage: string;
  sprint_index: string;
  valued_at?: string;
};

export async function getMyCompetitionStatus(
  competitionId: string,
  signal?: AbortSignal,
): Promise<CompetitionStatus> {
  const w = await apiRequest<MyStatusWire>(`/competitions/${competitionId}/me`, { signal });
  return {
    competitionId: w.competition_id,
    joined: w.joined,
    entryStatus: w.entry_status,
    currentRank: w.current_rank,
    returnPercentage: w.sprint_return_percentage,
    index: w.sprint_index,
    valuedAt: w.valued_at,
  };
}

type JoinWire = {
  competition_id: string;
  joined: boolean;
  entry_status?: string;
  eligibility?: EligibilityWire["rules"];
};

export async function joinCompetition(competitionId: string): Promise<JoinResult> {
  const w = await apiRequest<JoinWire>(`/competitions/${competitionId}/join`, {
    method: "POST",
    idempotencyKey: crypto.randomUUID(),
  });
  return {
    competitionId: w.competition_id,
    joined: w.joined,
    entryStatus: w.entry_status ?? "",
    eligibility: (w.eligibility ?? []).map((r) => ({
      code: r.code,
      label: r.label,
      required: r.required,
      actual: r.actual,
      passed: r.passed,
      reason: r.reason,
    })),
  };
}

export async function withdrawFromCompetition(competitionId: string): Promise<void> {
  await apiRequest(`/competitions/${competitionId}/entry`, { method: "DELETE" });
}

// The leaderboard route serves two different shapes from one endpoint: a
// plain array for legacy weekly sprints (SprintLeaderboardEntry[]), and a
// cursor-paginated object for engine editions (CompetitionLeaderboardPage).
// Both share row field names for rank/display_name/avatar_key; only the
// percentage/index field names and the envelope differ.
type LegacyLeaderboardRowWire = {
  rank: number;
  display_name: string;
  avatar_key: string;
  sprint_return_percentage: string;
  sprint_index: string;
};

type EngineLeaderboardRowWire = {
  rank: number;
  display_name: string;
  avatar_key: string;
  return_percentage: string;
  competition_index: string;
};

type EngineLeaderboardPageWire = {
  entries: EngineLeaderboardRowWire[];
  next_cursor?: number;
  valued_at?: string;
  unavailable?: boolean;
};

function toRow(
  w: LegacyLeaderboardRowWire | EngineLeaderboardRowWire,
  currentUserDisplayName: string | undefined,
): LeaderboardRow {
  const returnPercentage =
    "sprint_return_percentage" in w ? w.sprint_return_percentage : w.return_percentage;
  const index = "sprint_index" in w ? w.sprint_index : w.competition_index;
  return {
    rank: w.rank,
    displayName: w.display_name,
    avatarKey: w.avatar_key,
    returnPercentage,
    index,
    isCurrentUser: Boolean(
      currentUserDisplayName &&
        w.display_name.toLowerCase() === currentUserDisplayName.toLowerCase(),
    ),
  };
}

export async function getCompetitionLeaderboard(
  competitionId: string,
  options: { after?: number; limit?: number; currentUserDisplayName?: string } = {},
  signal?: AbortSignal,
): Promise<LeaderboardPage> {
  const params = new URLSearchParams();
  if (options.after !== undefined) params.set("after", String(options.after));
  if (options.limit !== undefined) params.set("limit", String(options.limit));
  const query = params.toString();
  const raw = await apiRequest<LegacyLeaderboardRowWire[] | EngineLeaderboardPageWire>(
    `/competitions/${competitionId}/leaderboard${query ? `?${query}` : ""}`,
    { signal },
  );
  if (Array.isArray(raw)) {
    return {
      entries: raw.map((row) => toRow(row, options.currentUserDisplayName)),
      unavailable: false,
    };
  }
  return {
    // The backend's "not ready" response omits no field but leaves entries
    // null (Go's encoding/json for a nil slice) — never assume it's an array.
    entries: (raw.entries ?? []).map((row) => toRow(row, options.currentUserDisplayName)),
    nextCursor: raw.next_cursor,
    valuedAt: raw.valued_at,
    unavailable: Boolean(raw.unavailable),
  };
}

type ResultRowWire = {
  rank: number;
  display_name: string;
  avatar_key: string;
  return_percentage: string;
  competition_index: string;
};

export async function getCompetitionResults(
  competitionId: string,
  currentUserDisplayName: string | undefined,
  signal?: AbortSignal,
): Promise<LeaderboardRow[]> {
  const rows = await apiRequest<ResultRowWire[]>(`/competitions/${competitionId}/results`, {
    signal,
  });
  return rows.map((row) => toRow(row, currentUserDisplayName));
}

export async function getAchievements(): Promise<Achievement[]> {
  const achievements = await apiRequest<LegacyAchievement[]>("/achievements/evaluate", {
    method: "POST",
  });
  return achievements.map((achievement) => ({
    id: achievement.key,
    name: achievement.name,
    description: achievement.description,
    currentProgress: achievement.unlocked ? 1 : 0,
    targetProgress: 1,
    isUnlocked: achievement.unlocked,
    unlockedAt: achievement.unlocked_at ?? undefined,
    difficulty: achievement.difficulty,
    period: achievement.period,
    inspiredBy: achievement.inspired_by,
    edgePoints: achievement.evidence?.edge_points,
  }));
}
