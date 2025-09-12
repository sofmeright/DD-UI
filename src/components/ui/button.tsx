// ui/src/components/ui/button.tsx
import React from "react";

type Props = React.ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "default" | "outline" | "ghost";
  size?: "sm" | "md" | "icon";
};

export function Button({ className="", variant="default", size="md", ...rest }: Props) {
  const base = "inline-flex items-center justify-center rounded-xl font-medium transition focus:outline-none focus-visible:ring-2 ring-offset-0";
  const v =
    variant === "outline"
      ? "border border-slate-700 text-slate-200 bg-transparent hover:bg-slate-800"
      : variant === "ghost"
      ? "bg-transparent hover:bg-slate-900"
      : "bg-slate-800 hover:bg-slate-700 text-white";
  const s =
    size === "sm" ? "h-8 px-3 text-sm"
    : size === "icon" ? "h-10 w-10"
    : "h-10 px-4";
  return <button className={`${base} ${v} ${s} ${className}`} {...rest} />;
}
