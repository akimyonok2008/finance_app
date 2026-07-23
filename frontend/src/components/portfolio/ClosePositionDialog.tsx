import { Loader2 } from "lucide-react";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { useClosePosition } from "@/hooks/usePositions";
import type { Position } from "@/types/portfolio";

type Props = {
  position: Position | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function ClosePositionDialog({ position, open, onOpenChange }: Props) {
  const closePosition = useClosePosition();
  const pending = closePosition.isPending;

  const handleConfirm = (e: React.MouseEvent) => {
    e.preventDefault();
    if (!position) return;
    closePosition.mutate(position.id, {
      onSuccess: () => onOpenChange(false),
    });
  };

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Close position</AlertDialogTitle>
          <AlertDialogDescription>
            Mark this asset as sold. Alarvest will lock the realized gain or
            loss at the latest available price. No trade is placed.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>Cancel</AlertDialogCancel>
          <AlertDialogAction onClick={handleConfirm} disabled={pending}>
            {pending ? (
              <>
                <Loader2 className="animate-spin" />
                Closing…
              </>
            ) : (
              "Close position"
            )}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
