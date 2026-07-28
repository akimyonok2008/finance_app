import { ShieldOff } from "lucide-react";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import { useBlockUser } from "@/hooks/useSafety";

type Props = {
  handle: string;
  displayName?: string;
};

/** Blocks a user, with a confirmation step. Backend enforcement (removing
 * follows, disabling messaging, hiding from search/Explore) happens
 * regardless of what this button does — this is just the UI trigger. */
export function BlockButton({ handle, displayName }: Props) {
  const block = useBlockUser();

  return (
    <AlertDialog>
      <AlertDialogTrigger asChild>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="border-rose-400/20 text-rose-200 hover:bg-rose-400/10 hover:text-rose-100"
        >
          <ShieldOff className="h-3.5 w-3.5" /> Block
        </Button>
      </AlertDialogTrigger>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Block {displayName ?? `@${handle}`}?</AlertDialogTitle>
          <AlertDialogDescription>
            They won't be able to follow or message you, and you won't see each other in search
            or Explore. This also removes any existing follow between you. You can unblock them
            later from Settings.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction onClick={() => block.mutate(handle)} disabled={block.isPending}>
            Block
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
