import { useState } from "react";
import { Plus } from "lucide-react";
import { useSearchParams } from "react-router-dom";

import { useAuth } from "@/auth/useAuth";
import { PortfolioCoachCard } from "@/components/coach/PortfolioCoachCard";
import { AppNav } from "@/components/layout/AppNav";
import { Button } from "@/components/ui/button";
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerHeader,
  DrawerTitle,
} from "@/components/ui/drawer";
import { AddPositionForm } from "@/components/portfolio/AddPositionForm";
import { DeletePositionDialog } from "@/components/portfolio/DeletePositionDialog";
import { EditPositionModal } from "@/components/portfolio/EditPositionModal";
import { PortfolioSummaryCards } from "@/components/portfolio/PortfolioSummaryCards";
import { PositionCardList } from "@/components/portfolio/PositionCardList";
import { PositionsTable } from "@/components/portfolio/PositionsTable";
import { usePositionRows, type PositionRow } from "@/hooks/usePositionRows";
import { cn } from "@/utils/cn";

const PORTFOLIO_TABS = ["positions", "coach"] as const;
type PortfolioTab = (typeof PORTFOLIO_TABS)[number];

export function PortfolioPage() {
  const { rows, isLoading, isError, error } = usePositionRows();
  const { user } = useAuth();
  const [searchParams, setSearchParams] = useSearchParams();
  const tabParam = searchParams.get("tab") as PortfolioTab | null;
  const activeTab =
    tabParam && PORTFOLIO_TABS.includes(tabParam) ? tabParam : "positions";

  const [editTarget, setEditTarget] = useState<PositionRow | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<PositionRow | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);

  const errorMessage = error?.message;
  const setActiveTab = (tab: PortfolioTab) => {
    const next = new URLSearchParams(searchParams);
    if (tab === "positions") {
      next.delete("tab");
    } else {
      next.set("tab", tab);
    }
    setSearchParams(next);
  };

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50">
      <main className="mx-auto w-full max-w-7xl px-4 py-4 sm:px-6 lg:px-8">
        <AppNav />

        <div className="mb-8 flex flex-col gap-1">
          <span className="text-xs font-medium text-zinc-500">Portfolio</span>
          <h1 className="text-2xl font-medium tracking-tight sm:text-3xl">
            Your portfolio
          </h1>
          <p className="text-sm text-muted-foreground">
            {user?.display_name
              ? `${user.display_name}'s holdings and private coach analysis.`
              : "Track your holdings and review private portfolio analysis."}
          </p>
        </div>

        <div className="mb-6 flex flex-wrap gap-2 rounded-xl border border-zinc-800 bg-zinc-900/35 p-1">
          {PORTFOLIO_TABS.map((tab) => (
            <button
              key={tab}
              type="button"
              onClick={() => setActiveTab(tab)}
              className={cn(
                "rounded-lg px-3 py-2 text-sm font-medium capitalize transition",
                activeTab === tab
                  ? "bg-zinc-100 text-zinc-950"
                  : "text-zinc-400 hover:bg-zinc-800/70 hover:text-zinc-100",
              )}
            >
              {tab}
            </button>
          ))}
        </div>

        <PortfolioSummaryCards />

        {activeTab === "positions" ? (
          <div className="mt-8 grid gap-6 lg:grid-cols-[420px_1fr]">
            <div className="hidden lg:block">
              <AddPositionForm />
            </div>

            <div>
              <PositionsTable
                rows={rows}
                isLoading={isLoading}
                isError={isError}
                errorMessage={errorMessage}
                onEdit={setEditTarget}
                onDelete={setDeleteTarget}
              />
              <PositionCardList
                rows={rows}
                isLoading={isLoading}
                isError={isError}
                errorMessage={errorMessage}
                onEdit={setEditTarget}
                onDelete={setDeleteTarget}
                onAdd={() => setDrawerOpen(true)}
              />
            </div>
          </div>
        ) : (
          <div className="mt-8 mx-auto max-w-3xl">
            <div className="mb-5">
              <div className="flex items-center gap-3">
                <h2 className="text-lg font-semibold text-zinc-100">
                  Portfolio Coach
                </h2>
                <span className="rounded-md border border-zinc-800 bg-zinc-900/60 px-2 py-1 text-[11px] text-zinc-500">
                  Analysis only
                </span>
              </div>
              <p className="mt-1 text-sm text-zinc-400">
                Private analysis of the holdings on this Portfolio page.
              </p>
            </div>
            <PortfolioCoachCard />
          </div>
        )}
      </main>

      <div
        className={cn(
          "fixed inset-x-0 bottom-0 z-30 justify-center pb-[max(1rem,env(safe-area-inset-bottom))] lg:hidden",
          activeTab === "positions" ? "flex" : "hidden",
        )}
      >
        <Button
          variant="accent"
          size="lg"
          className="border border-zinc-700 shadow-lg shadow-black/30"
          onClick={() => setDrawerOpen(true)}
        >
          <Plus />
          Add Position
        </Button>
      </div>

      <Drawer open={drawerOpen} onOpenChange={setDrawerOpen}>
        <DrawerContent>
          <DrawerHeader>
            <DrawerTitle>Add Position</DrawerTitle>
            <DrawerDescription>
              Add a holding to track.
            </DrawerDescription>
          </DrawerHeader>
          <div className="overflow-y-auto px-4 pb-[max(1.5rem,env(safe-area-inset-bottom))]">
            <AddPositionForm compact onSuccess={() => setDrawerOpen(false)} />
          </div>
        </DrawerContent>
      </Drawer>

      <EditPositionModal
        position={editTarget}
        open={editTarget !== null}
        onOpenChange={(open) => !open && setEditTarget(null)}
      />
      <DeletePositionDialog
        position={deleteTarget}
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      />
    </div>
  );
}
