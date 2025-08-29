import React from "react";

export function Switch({ checked, onCheckedChange }: { checked: boolean; onCheckedChange: (v:boolean)=>void }) {
  return (
    <button
      role="switch"
      aria-checked={checked}
      onClick={()=>onCheckedChange(!checked)}
      className={`h-6 w-10 rounded-full border border-slate-700 transition ${checked ? "bg-brand/70" : "bg-slate-800"}`}>
      <span className={`block h-5 w-5 rounded-full bg-white transition transform ${checked ? "translate-x-4" : "translate-x-0.5"} mt-0.5 ml-0.5`} />
    </button>
  );
}
