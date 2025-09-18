// ui/src/components/ui/tabs.tsx
import React, { createContext, useContext, useState } from "react";

type Ctx = { value: string; setValue: (v:string)=>void };
const TabsCtx = createContext<Ctx | null>(null);

export function Tabs({ defaultValue, children, className="" }: React.PropsWithChildren<{defaultValue: string; className?: string}>) {
  const [value, setValue] = useState(defaultValue);
  return <TabsCtx.Provider value={{value,setValue}}><div className={className}>{children}</div></TabsCtx.Provider>;
}

export function TabsList({ children, className="" }: React.PropsWithChildren<{className?: string}>) {
  return <div className={`inline-flex rounded-xl overflow-hidden ${className}`}>{children}</div>;
}

export function TabsTrigger({ value, children }: React.PropsWithChildren<{value: string}>) {
  const ctx = useContext(TabsCtx)!;
  const active = ctx.value === value;
  return (
    <button type="button"
      className={`px-4 py-2 text-sm border ${active ? "bg-slate-800 text-white border-slate-700" : "bg-slate-900 text-slate-300 border-slate-800 hover:bg-slate-800"}`}
      onClick={()=>ctx.setValue(value)}
    >{children}</button>
  );
}

export function TabsContent({ value, children, className="" }: React.PropsWithChildren<{value: string; className?: string}>) {
  const ctx = useContext(TabsCtx)!;
  if (ctx.value !== value) return null;
  return <div className={className}>{children}</div>;
}
