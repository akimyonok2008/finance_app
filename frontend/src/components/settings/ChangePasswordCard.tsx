import { zodResolver } from "@hookform/resolvers/zod";
import { LoaderCircle } from "lucide-react";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useChangePassword } from "@/hooks/useAccount";

const schema = z
  .object({
    current_password: z.string().min(1, "Current password is required"),
    new_password: z.string().min(8, "New password must be at least 8 characters"),
    confirm_password: z.string().min(1, "Confirm your new password"),
  })
  .refine((values) => values.new_password === values.confirm_password, {
    message: "Passwords do not match",
    path: ["confirm_password"],
  });

type Values = z.infer<typeof schema>;

function FieldError({ message }: { message?: string }) {
  return message ? <p className="mt-1.5 text-xs text-rose-300">{message}</p> : null;
}

export function ChangePasswordCard() {
  const changePassword = useChangePassword();
  const {
    register,
    handleSubmit,
    reset,
    setError,
    formState: { errors },
  } = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: { current_password: "", new_password: "", confirm_password: "" },
  });

  const onSubmit = (values: Values) => {
    changePassword.mutate(
      { currentPassword: values.current_password, newPassword: values.new_password },
      {
        onSuccess: () => reset(),
        onError: (err: Error) => {
          setError("current_password", { message: err.message });
        },
      },
    );
  };

  return (
    <form
      onSubmit={handleSubmit(onSubmit)}
      className="rounded-2xl border border-violet-300/15 bg-gradient-to-br from-violet-400/[0.055] to-zinc-900/70 p-5 shadow-lg shadow-violet-950/10 sm:p-6"
    >
      <h2 className="text-base font-semibold text-zinc-50">Change password</h2>
      <p className="mt-1 text-xs text-zinc-400">
        You&apos;ll need your current password to set a new one.
      </p>

      <div className="mt-5 grid gap-4 sm:grid-cols-2">
        <div className="sm:col-span-2">
          <Label htmlFor="current_password">Current password</Label>
          <Input
            id="current_password"
            type="password"
            autoComplete="current-password"
            className="mt-2"
            aria-invalid={!!errors.current_password}
            {...register("current_password")}
          />
          <FieldError message={errors.current_password?.message} />
        </div>
        <div>
          <Label htmlFor="new_password">New password</Label>
          <Input
            id="new_password"
            type="password"
            autoComplete="new-password"
            className="mt-2"
            aria-invalid={!!errors.new_password}
            {...register("new_password")}
          />
          <FieldError message={errors.new_password?.message} />
        </div>
        <div>
          <Label htmlFor="confirm_password">Confirm new password</Label>
          <Input
            id="confirm_password"
            type="password"
            autoComplete="new-password"
            className="mt-2"
            aria-invalid={!!errors.confirm_password}
            {...register("confirm_password")}
          />
          <FieldError message={errors.confirm_password?.message} />
        </div>
      </div>

      <Button type="submit" className="mt-5 w-full bg-violet-200 text-zinc-950 hover:bg-violet-100" disabled={changePassword.isPending}>
        {changePassword.isPending ? <LoaderCircle className="animate-spin" /> : null}
        {changePassword.isPending ? "Updating password" : "Update password"}
      </Button>
    </form>
  );
}
