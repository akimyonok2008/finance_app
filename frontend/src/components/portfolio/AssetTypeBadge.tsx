import { Badge } from "@/components/ui/badge";
import type { AssetType } from "@/types/portfolio";

const LABELS: Record<AssetType, string> = {
  stock: "Stock",
  etf: "ETF",
  crypto: "Crypto",
};

export function AssetTypeBadge({ type }: { type: AssetType }) {
  const variant = type === "crypto" ? "crypto" : "default";
  return <Badge variant={variant}>{LABELS[type] ?? type}</Badge>;
}
