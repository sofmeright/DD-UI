import * as React from "react";
import { cn } from "@/lib/utils";

export function Separator({ className, orientation = "horizontal" as "horizontal" | "vertical" }) {
  return (
    <div
      className={cn(
        "bg-slate-800",
        orientation === "horizontal" ? "h-px w-full" : "w-px h-full",
        className
      )}
    />
  );
}
