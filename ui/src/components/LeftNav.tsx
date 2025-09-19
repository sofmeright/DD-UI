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
  onGoLogging,
  onGoGitSync,
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
  onGoLogging?: () => void;
  onGoGitSync?: () => void;
}) {
  const item = (id: string, label: string, onClick: () => void) => (
    <button
      className={`w-full text-left px-3 py-1 xl:py-1.5 rounded-lg text-sm transition border ${
        page === id ? "bg-slate-800/60 border-slate-700 text-white" : "hover:bg-slate-900/40 border-transparent text-slate-300"
      }`}
      onClick={onClick}
    >
      {label}
    </button>
  );

  return (
    <div className="hidden md:flex md:flex-col w-52 shrink-0 border-r border-slate-800 bg-slate-950/60">
      <div className="px-4 py-2.5 border-b border-slate-800">
        <div className="flex flex-col items-center">
          <div className="font-black uppercase tracking-tight leading-none text-slate-200 select-none text-3xl xl:text-5xl mb-0.5">
            <span className="gradient-wave">
              DD-UI
            </span>
          </div>
          <div className="flex justify-center mb-0.5">
            <Badge variant="outline" className="w-fit text-xs">
              Community Edition
            </Badge>
          </div>
          <img src="/DD-UI-Logo.png" alt="DD-UI" className="h-16 w-16 xl:h-24 xl:w-24 rounded-md" />
        </div>
      </div>

      {onGoDashboard && (
        <>
          <div className="px-4 py-1 xl:py-1.5 text-xs tracking-wide uppercase text-slate-400">
            Overview
          </div>
          <nav className="px-2 pb-0.5 xl:pb-2 space-y-0.5 xl:space-y-1">
            {item("dashboard", "Dashboard", onGoDashboard)}
            {onGoGitSync && item("git", "GitOps", onGoGitSync)}
          </nav>
        </>
      )}

      <div className="px-4 py-1 xl:py-1.5 text-xs tracking-wide uppercase text-slate-400">
        Infrastructure
      </div>
      <nav className="px-2 pb-0.5 xl:pb-2 space-y-0.5 xl:space-y-1">
        {onGoGroups && item("groups", "Groups", onGoGroups)}
        {item("hosts", "Hosts", onGoHosts)}
        {item("stacks", "Stacks", onGoStacks)}
      </nav>

      <div className="px-4 py-1 xl:py-1.5 text-xs tracking-wide uppercase text-slate-400">
        Resources
      </div>
      <nav className="px-2 pb-0.5 xl:pb-2 space-y-0.5 xl:space-y-1">
        {item("images", "Images", onGoImages)}
        {item("networks", "Networks", onGoNetworks)}
        {item("volumes", "Volumes", onGoVolumes)}
      </nav>

      <div className="px-4 py-1 xl:py-1.5 text-xs tracking-wide uppercase text-slate-400">
        Operations
      </div>
      <nav className="px-2 pb-0.5 xl:pb-2 space-y-0.5 xl:space-y-1">
        {onGoCleanup && item("cleanup", "Cleanup", onGoCleanup)}
        {onGoLogging && item("logging", "Logging", onGoLogging)}
      </nav>

      <div className="px-4 py-1 xl:py-1.5 text-xs tracking-wide uppercase text-slate-400 hidden xl:block">
        System
      </div>
      <nav className="px-2 space-y-0.5 xl:space-y-1">
        <div className="px-3 py-1 xl:py-1.5 text-slate-300 text-sm hidden xl:block">Settings</div>
        <div className="px-3 py-1 xl:py-1.5 text-slate-300 text-sm hidden xl:block">About</div>
        <div className="px-3 py-1 xl:py-1.5 text-slate-300 text-sm hidden xl:block">Help</div>
        <form method="post" action="/logout" className="xl:mt-0 -mt-1">
          <Button
            type="submit"
            variant="ghost"
            className="px-3 py-1 xl:py-2 text-slate-300 hover:bg-slate-900/60 text-sm"
          >
            Logout
          </Button>
        </form>
      </nav>
    </div>
  );
}