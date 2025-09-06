// ui/src/components/ui/separator.tsx
import React from "react";

export function Separator({ orientation="horizontal", className="" }: {orientation?: "horizontal"|"vertical"; className?: string}) {
  return <div className={`${orientation==="vertical" ? "w-px" : "h-px"} bg-slate-800 ${className}`} />;
}
