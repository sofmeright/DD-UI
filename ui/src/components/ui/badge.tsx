import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const badgeVariants = cva(
  "inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-medium",
  {
    variants: {
      variant: {
        default: "border-transparent bg-slate-800 text-slate-100",
        outline: "border-slate-700/60 text-slate-300 bg-transparent"
      }
    },
    defaultVariants: { variant: "default" }
  }
);

export interface BadgeProps extends React.HTMLAttributes<HTMLDivElement>, VariantProps<typeof badgeVariants> {}
export const Badge = ({ className, variant, ...props }: BadgeProps) => (
  <div className={cn(badgeVariants({ variant }), className)} {...props} />
);
