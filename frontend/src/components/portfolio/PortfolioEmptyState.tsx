import { motion } from "framer-motion";
import { LineChart, Plus } from "lucide-react";

import { Button } from "@/components/ui/button";

type Props = {
  /** When provided (mobile), shows an "Add Position" button that opens the drawer. */
  onAdd?: () => void;
};

export function PortfolioEmptyState({ onAdd }: Props) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.18 }}
      className="flex flex-col items-center justify-center gap-4 px-6 py-16 text-center"
    >
      <div className="grid h-12 w-12 place-items-center rounded-xl border border-zinc-800 bg-zinc-900/50 text-zinc-400">
        <LineChart className="h-6 w-6" />
      </div>
      <div className="space-y-1.5">
        <h3 className="text-lg font-semibold tracking-tight">
          No positions yet
        </h3>
        <p className="mx-auto max-w-sm text-sm text-muted-foreground">
          Add your first position to begin.
        </p>
      </div>
      {onAdd && (
        <Button variant="accent" onClick={onAdd} className="mt-1">
          <Plus />
          Add Position
        </Button>
      )}
    </motion.div>
  );
}
