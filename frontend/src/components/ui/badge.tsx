import { type VariantProps } from "class-variance-authority";

import { cn } from "@/utils/cn";
import { badgeVariants } from "@/components/ui/badge-variants";

export interface BadgeProps
  extends React.HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return (
    <span className={cn(badgeVariants({ variant }), className)} {...props} />
  );
}

export { Badge };
