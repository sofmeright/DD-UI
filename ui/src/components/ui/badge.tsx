import React from "react";

export function Badge({ className="", variant="default", children }: React.PropsWithChildren<{className?: string; variant?: "default"|"outline"}>) {
  const v = variant === "outline"
    ? "border border-slate-700/60 text-slate-300"
    : "bg-slate-800 text-slate-200";
  return <span className={`inline-flex items-center gap-1 px-2 py-1 rounded-md text-[11px] ${v} ${className}`}>{children}</span>;
}
