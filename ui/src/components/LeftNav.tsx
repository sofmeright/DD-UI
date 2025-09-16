// ui/src/components/LeftNav.tsx
import React from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

export default function LeftNav({
  page,
  onGoHosts,
  onGoStacks,
  onGoImages,
  onGoNetworks,
  onGoVolumes,
  onGoDashboard,
  onGoGroups,
  onGoCleanup,
}: {
  page: string;
  onGoHosts: () => void;
  onGoStacks: () => void;
  onGoImages: () => void;
  onGoNetworks: () => void;
  onGoVolumes: () => void;
  onGoDashboard?: () => void;
  onGoGroups?: () => void;
  onGoCleanup?: () => void;
}) {
  const item = (id: string, label: string, onClick: () => void) => (
    <button
      className={`w-full text-left px-3 py-2 rounded-lg text-sm transition border ${
        page === id
          ? "bg-slate-800/60 border-slate-700 text-white"
          : "hover:bg-slate-900/40 border-transparent text-slate-300"
      }`}
      onClick={onClick}
    >
      {label}
    </button>
  );

  return (
    <div className="hidden md:flex md:flex-col w-52 shrink-0 border-r border-slate-800 bg-slate-950/60">
      <div className="px-4 py-4 border-b border-slate-800">
        <div className="flex flex-col items-center">
          <div className="font-black uppercase tracking-tight leading-none text-slate-200 select-none text-5xl mb-1">
            <span className="gradient-wave">
              DD-UI
            </span>
          </div>
          <div className="flex justify-center mb-1">
            <Badge variant="outline" className="w-fit text-xs">
              Community Edition
            </Badge>
          </div>
          <img src="/DDUI-Logo.png" alt="DD-UI" className="h-28 w-28 rounded-md" />
        </div>
      </div>

      {onGoDashboard && (
        <>
          <div className="px-4 py-3 text-xs tracking-wide uppercase text-slate-400">
            Overview
          </div>
          <nav className="px-2 pb-4 space-y-1">
            {item("dashboard", "Dashboard", onGoDashboard)}
          </nav>
        </>
      )}

      <div className="px-4 py-3 text-xs tracking-wide uppercase text-slate-400">
        Infrastructure
      </div>
      <nav className="px-2 pb-4 space-y-1">
        {item("hosts", "Hosts", onGoHosts)}
        {item("stacks", "Stacks", onGoStacks)}
        {onGoGroups && item("groups", "Groups", onGoGroups)}
      </nav>

      <div className="px-4 py-3 text-xs tracking-wide uppercase text-slate-400">
        Resources
      </div>
      <nav className="px-2 pb-4 space-y-1">
        {item("images", "Images", onGoImages)}
        {item("networks", "Networks", onGoNetworks)}
        {item("volumes", "Volumes", onGoVolumes)}
      </nav>

      <div className="px-4 py-3 text-xs tracking-wide uppercase text-slate-400">
        Operations
      </div>
      <nav className="px-2 pb-4 space-y-1">
        {onGoCleanup && item("cleanup", "Cleanup", onGoCleanup)}
      </nav>

      <div className="px-4 py-3 text-xs tracking-wide uppercase text-slate-400">
        System
      </div>
      <nav className="px-2 space-y-1">
        <div className="px-3 py-2 text-slate-300 text-sm">Settings</div>
        <div className="px-3 py-2 text-slate-300 text-sm">About</div>
        <div className="px-3 py-2 text-slate-300 text-sm">Help</div>
        <form method="post" action="/logout">
          <Button
            type="submit"
            variant="ghost"
            className="px-3 text-slate-300 hover:bg-slate-900/60"
          >
            Logout
          </Button>
        </form>
      </nav>
    </div>
  );
}