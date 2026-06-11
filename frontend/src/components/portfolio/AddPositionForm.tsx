import { useState } from "react";
import { AlertCircle, Loader2, Plus } from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { PositionFormFields } from "@/components/portfolio/PositionFormFields";
import {
  EMPTY_POSITION_FORM,
  validatePositionForm,
  type PositionFormErrors,
  type PositionFormState,
} from "@/components/portfolio/positionForm";
import { useCreatePosition } from "@/hooks/usePositions";
import { cn } from "@/utils/cn";

export type AddPositionFormProps = {
  /** Called after a successful add (e.g. to close the mobile drawer). */
  onSuccess?: () => void;
  /** Render without the surrounding Card chrome (used inside the drawer). */
  compact?: boolean;
};

export function AddPositionForm({ onSuccess, compact }: AddPositionFormProps) {
  const [state, setState] = useState<PositionFormState>(EMPTY_POSITION_FORM);
  const [errors, setErrors] = useState<PositionFormErrors>({});
  const [backendError, setBackendError] = useState<string | null>(null);

  const createPosition = useCreatePosition();

  const onChange = (patch: Partial<PositionFormState>) => {
    setState((prev) => ({ ...prev, ...patch }));
    // Clear the field-level error as the user edits it.
    setErrors((prev) => {
      const next = { ...prev };
      for (const key of Object.keys(patch)) {
        delete next[key as keyof PositionFormState];
      }
      return next;
    });
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setBackendError(null);

    const result = validatePositionForm(state);
    if (!result.ok) {
      setErrors(result.errors);
      return;
    }
    setErrors({});

    createPosition.mutate(result.value, {
      onSuccess: () => {
        // Clear the form, then let the parent close the drawer if mobile.
        setState(EMPTY_POSITION_FORM);
        onSuccess?.();
      },
      onError: (err: Error) => {
        // Keep the user's values; surface the backend message inline + toast.
        setBackendError(err.message);
      },
    });
  };

  const pending = createPosition.isPending;

  const formBody = (
    <form onSubmit={handleSubmit} className="grid gap-5" noValidate>
      {backendError && (
        <div
          role="alert"
          className="flex items-start gap-2 rounded-xl border border-rose-500/30 bg-rose-500/10 px-3 py-2.5 text-sm text-rose-200"
        >
          <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
          <span>{backendError}</span>
        </div>
      )}

      <PositionFormFields
        idPrefix="add"
        state={state}
        errors={errors}
        disabled={pending}
        onChange={onChange}
      />

      <Button
        type="submit"
        variant="accent"
        className="w-full"
        disabled={pending}
      >
        {pending ? (
          <>
            <Loader2 className="animate-spin" />
            Adding…
          </>
        ) : (
          <>
            <Plus />
            Add Position
          </>
        )}
      </Button>
    </form>
  );

  if (compact) {
    return formBody;
  }

  return (
    <Card className={cn("sticky top-6")}>
      <CardHeader>
        <CardTitle>Add Position</CardTitle>
        <CardDescription>
          Add a holding to track.
        </CardDescription>
      </CardHeader>
      <CardContent>{formBody}</CardContent>
    </Card>
  );
}
