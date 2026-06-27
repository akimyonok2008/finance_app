import { apiRequest } from "@/api/client";
import type {
  Concentration,
  Exposure,
  MyProfile,
  ProfileBadge,
  PublicProfile,
  PublicWeight,
  UpdateProfileRequest,
} from "@/types/profile";

type UnknownRecord = Record<string, unknown>;

const record = (value: unknown): UnknownRecord =>
  value && typeof value === "object" && !Array.isArray(value)
    ? (value as UnknownRecord)
    : {};

const stringValue = (value: unknown): string | undefined =>
  typeof value === "string" ? value : undefined;

const numberValue = (value: unknown): number | undefined =>
  typeof value === "number" && Number.isFinite(value) ? value : undefined;

const booleanValue = (value: unknown): boolean =>
  typeof value === "boolean" ? value : false;

function normalizeBadge(value: unknown): ProfileBadge | null {
  const raw = record(value);
  const key = stringValue(raw.key);
  const name = stringValue(raw.name);
  if (!key || !name) return null;
  return {
    key,
    name,
    icon_key: stringValue(raw.icon_key) ?? stringValue(raw.icon),
    unlocked_at: stringValue(raw.unlocked_at),
  };
}

function normalizeWeight(value: unknown): PublicWeight | null {
  const raw = record(value);
  const symbol = stringValue(raw.symbol);
  const weight = numberValue(raw.weight_percentage) ?? numberValue(raw.weight);
  if (!symbol || weight === undefined) return null;
  return {
    symbol,
    weight_percentage: weight,
    asset_type: stringValue(raw.asset_type),
  };
}

function normalizeExposure(value: unknown): Exposure | null {
  const raw = record(value);
  const name =
    stringValue(raw.name) ??
    stringValue(raw.asset_type) ??
    stringValue(raw.currency);
  const weight = numberValue(raw.weight_percentage) ?? numberValue(raw.weight);
  if (!name || weight === undefined) return null;
  return { name, weight_percentage: weight };
}

function normalizeConcentration(value: unknown): Concentration | undefined {
  const raw = record(value);
  const concentration = {
    position_count: numberValue(raw.position_count),
    largest_weight_percentage:
      numberValue(raw.largest_weight_percentage) ??
      numberValue(raw.largest_position),
    top3_weight_percentage:
      numberValue(raw.top3_weight_percentage) ?? numberValue(raw.top_three),
  };
  return Object.values(concentration).some((item) => item !== undefined)
    ? concentration
    : undefined;
}

export function normalizePublicProfile(value: unknown): PublicProfile {
  const raw = record(value);
  const weights = Array.isArray(raw.public_weights)
    ? raw.public_weights
        .map(normalizeWeight)
        .filter((item): item is PublicWeight => item !== null)
        .sort((a, b) => b.weight_percentage - a.weight_percentage)
    : [];
  const concentration = normalizeConcentration(raw.concentration);

  return {
    handle: stringValue(raw.handle) ?? "",
    display_name: stringValue(raw.display_name) ?? "Investor",
    avatar_key: stringValue(raw.avatar_key),
    bio: stringValue(raw.bio),
    strategy_tag: stringValue(raw.strategy_tag),
    joined_at: stringValue(raw.joined_at),
    portfolio_index: numberValue(raw.portfolio_index),
    return_percentage: numberValue(raw.return_percentage),
    global_rank: numberValue(raw.global_rank) ?? null,
    sprint_rank: numberValue(raw.sprint_rank) ?? null,
    badges: Array.isArray(raw.badges)
      ? raw.badges
          .map(normalizeBadge)
          .filter((item): item is ProfileBadge => item !== null)
      : [],
    public_weights: weights,
    asset_type_exposure: Array.isArray(raw.asset_type_exposure)
      ? raw.asset_type_exposure
          .map(normalizeExposure)
          .filter((item): item is Exposure => item !== null)
      : [],
    currency_exposure: Array.isArray(raw.currency_exposure)
      ? raw.currency_exposure
          .map(normalizeExposure)
          .filter((item): item is Exposure => item !== null)
      : [],
    concentration:
      concentration || weights.length > 0
        ? {
            ...concentration,
            position_count:
              concentration?.position_count ??
              (weights.length > 0 ? weights.length : undefined),
          }
        : undefined,
  };
}

function normalizeMyProfile(value: unknown): MyProfile {
  const raw = record(value);
  return {
    handle: stringValue(raw.handle) ?? "",
    display_name: stringValue(raw.display_name) ?? "Investor",
    avatar_key: stringValue(raw.avatar_key),
    bio: stringValue(raw.bio),
    strategy_tag: stringValue(raw.strategy_tag),
    is_public: booleanValue(raw.is_public),
    show_public_weights: booleanValue(raw.show_public_weights),
    created_at: stringValue(raw.created_at),
    updated_at: stringValue(raw.updated_at),
    public_preview: normalizePublicProfile(raw.public_preview),
  };
}

export async function getMyProfile(signal?: AbortSignal): Promise<MyProfile> {
  return normalizeMyProfile(
    await apiRequest<unknown>("/profiles/me", { signal }),
  );
}

export async function updateMyProfile(
  input: UpdateProfileRequest,
): Promise<MyProfile> {
  return normalizeMyProfile(
    await apiRequest<unknown>("/profiles/me", { method: "PATCH", body: input }),
  );
}

export async function getPublicProfile(
  handle: string,
  signal?: AbortSignal,
): Promise<PublicProfile> {
  return normalizePublicProfile(
    await apiRequest<unknown>(`/profiles/${encodeURIComponent(handle)}`, {
      signal,
    }),
  );
}
