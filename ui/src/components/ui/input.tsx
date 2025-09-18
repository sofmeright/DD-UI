// ui/src/components/ui/input.tsx
import React from "react";

export function Input(props: React.InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={`h-10 w-full rounded-xl border border-slate-800 bg-slate-900/60 px-3 text-sm outline-none focus:ring-2 focus:ring-brand ${props.className||""}`} />;
}
