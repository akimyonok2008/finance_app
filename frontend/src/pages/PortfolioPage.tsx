import { useState } from "react";
import { Plus } from "lucide-react";

import { useAuth } from "@/auth/useAuth";
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

export function PortfolioPage() {
  const { rows, isLoading, isError, error } = usePositionRows();
  const { user } = useAuth();

  const [editTarget, setEditTarget] = useState<PositionRow | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<PositionRow | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);

  const errorMessage = error?.message;

  return (
    <div className="min-h-screen bg-zinc-950 text-zinc-50">
      <main className="mx-auto w-full max-w-7xl px-4 py-4 sm:px-6 lg:px-8">
        <AppNav />

        {/* Header */}
        <div className="mb-8 flex flex-col gap-1">
          <span className="text-xs font-medium text-zinc-500">Portfolio</span>
          <h1 className="text-2xl font-medium tracking-tight sm:text-3xl">
            Your positions
          </h1>
          <p className="text-sm text-muted-foreground">
            {user?.display_name
              ? `${user.display_name}'s holdings`
              : "Track your holdings and performance."}
          </p>
        </div>

        <PortfolioSummaryCards />

        <div className="mt-8 grid gap-6 lg:grid-cols-[420px_1fr]">
          {/* Desktop add form */}
          <div className="hidden lg:block">
            <AddPositionForm />
          </div>

          {/* Positions */}
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
      </main>

      {/* Mobile floating add button */}
      <div className="fixed inset-x-0 bottom-0 z-30 flex justify-center pb-[max(1rem,env(safe-area-inset-bottom))] lg:hidden">
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

      {/* Mobile add drawer */}
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

      {/* Edit + delete dialogs */}
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
