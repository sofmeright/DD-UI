// ui/src/components/ui/badge.tsx
import React from "react";

type Variant = "default" | "outline" | "destructive" | "success";

export function Badge({
  className = "",
  variant = "default",
  children,
}: React.PropsWithChildren<{ className?: string; variant?: Variant }>) {
  const v =
    variant === "outline" ?
      "border border-slate-700/60 text-slate-300" :
      (variant === "destructive" ?
        "bg-rose-900/40 border border-rose-700/40 text-rose-200" :
        (variant === "success" ?
          "bg-emerald-900/40 border border-emerald-700/40 text-emerald-200" :
          "bg-slate-800 text-slate-200"));
  return (
    <span className={`inline-flex items-center gap-1 px-2 py-1 rounded-md text-[11px] ${v} ${className}`}>
      {children}
    </span>
  );
}
