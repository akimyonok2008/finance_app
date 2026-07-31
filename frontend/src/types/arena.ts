// Arena deliberately shows rank and percentage performance only. Backend DTOs
// never carry portfolio values, holdings, quantities, symbols, average
// purchase prices, cash balances, email, user id, or portfolio id.
//
// Percentage/index fields are the backend's authoritative decimal-string
// contract (see src/utils/decimal.ts) — they arrive as strings like
// "12.34", never as JS numbers, and must only be converted at a formatting
// boundary (formatPercent / decimalToChartNumber), never for arithmetic.
import type { DecimalString } from "@/utils/decimal";

/** One catalogue card: a legacy weekly sprint OR an engine competition edition. */
export type ArenaCompetitionCard = {
  id: string;
  name: string;
  description?: string;
  category?: string;
  iconKey?: string;
  /** Raw backend status: legacy derives upcoming/active/completed; engine editions report their stored lifecycle (draft/published/registration_open/registration_closed/active/finalizing/completed/cancelled). */
  status: string;
  startsAt: string;
  endsAt: string;
  joinOpensAt?: string;
  joinClosesAt?: string;
  scoringScope?: string;
  participantCount: number;
  joined: boolean;
  entryStatus?: string;
  /** Pre-engine weekly sprint: join-time baseline + live repricing, kept only for migration compatibility with users already entered in one. */
  isLegacy: boolean;
};

export type EligibilityRuleResult = {
  code: string;
  label: string;
  required: string;
  actual: string;
  passed: boolean;
  reason?: string;
};

export type EligibilityPreview = {
  eligible: boolean;
  evaluatedAt: string;
  rules: EligibilityRuleResult[];
};

export type CompetitionStatus = {
  competitionId: string;
  joined: boolean;
  entryStatus?: string;
  currentRank: number;
  returnPercentage: DecimalString;
  index: DecimalString;
  valuedAt?: string;
};

export type LeaderboardRow = {
  rank: number;
  displayName: string;
  avatarKey?: string;
  returnPercentage: DecimalString;
  index: DecimalString;
  isCurrentUser: boolean;
};

export type LeaderboardPage = {
  entries: LeaderboardRow[];
  returnModel?: "fixed_basket_price_return_v1";
  nextCursor?: number;
  valuedAt?: string;
  /** No ranking generation has ever been promoted yet — a controlled "not ready", never a live scan. */
  unavailable: boolean;
};

export type JoinResult = {
  competitionId: string;
  joined: boolean;
  entryStatus: string;
  eligibility: EligibilityRuleResult[];
};

export type Achievement = {
  id: string;
  name: string;
  description: string;
  currentProgress: number;
  targetProgress: number;
  isUnlocked: boolean;
  unlockedAt?: string;
  difficulty?: string;
  period?: string;
  inspiredBy?: string;
  edgePoints?: number;
};
