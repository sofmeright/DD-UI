import * as React from "react";
import { cn } from "@/lib/utils";

export interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {}

export const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ className, type, ...props }, ref) => (
    <input
      type={type}
      className={cn(
        "flex h-9 w-full rounded-md border border-slate-800 bg-slate-900/60 px-3 py-1 text-sm text-slate-200 placeholder:text-slate-500 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[#74ecbe]",
        className
      )}
      ref={ref}
      {...props}
    />
  )
);
Input.displayName = "Input";
