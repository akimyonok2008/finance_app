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
import { useDeletePosition } from "@/hooks/usePositions";
import type { Position } from "@/types/portfolio";

type Props = {
  position: Position | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function DeletePositionDialog({ position, open, onOpenChange }: Props) {
  const deletePosition = useDeletePosition();
  const pending = deletePosition.isPending;

  const handleConfirm = (e: React.MouseEvent) => {
    // Keep the dialog open until the request resolves, then close on success.
    e.preventDefault();
    if (!position) return;
    deletePosition.mutate(position.id, {
      onSuccess: () => onOpenChange(false),
    });
  };

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete position?</AlertDialogTitle>
          <AlertDialogDescription>
            This removes
            {position ? ` ${position.symbol}` : " the position"} from your
            portfolio. This action cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>Cancel</AlertDialogCancel>
          <AlertDialogAction onClick={handleConfirm} disabled={pending}>
            {pending ? (
              <>
                <Loader2 className="animate-spin" />
                Deleting…
              </>
            ) : (
              "Delete position"
            )}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
