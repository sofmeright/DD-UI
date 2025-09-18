// ui/src/components/ui/card.tsx
import React from "react";

export const Card = React.forwardRef<HTMLDivElement, React.PropsWithChildren<{className?: string}>>(
  ({ className="", children }, ref) => {
    return <div ref={ref} className={`rounded-2xl border border-slate-800 ${className}`}>{children}</div>;
  }
);
export function CardHeader({ className="", children }: React.PropsWithChildren<{className?: string}>) {
  return <div className={`px-4 pt-4 ${className}`}>{children}</div>;
}
export function CardTitle({ className="", children }: React.PropsWithChildren<{className?: string}>) {
  return <div className={`text-lg font-semibold ${className}`}>{children}</div>;
}
export function CardContent({ className="", children }: React.PropsWithChildren<{className?: string}>) {
  return <div className={`px-4 pb-4 ${className}`}>{children}</div>;
}