import { Host } from "@/types";
import { handle401 } from "@/utils/auth";
import GitSyncToggle from "@/components/GitSyncToggle";

export default function GroupsView({ hosts }: { hosts: Host[] }) {
  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center gap-4">
        <div className="text-lg font-semibold text-white">Groups</div>
        
        {/* Git sync toggle positioned at the end */}
        <div className="ml-auto">
          <GitSyncToggle />
        </div>
      </div>

      {/* Coming Soon Placeholder */}
      <div className="flex items-center justify-center min-h-[400px]">
        <div className="text-center space-y-4">
          <div className="text-6xl">👥</div>
          <div className="text-xl font-semibold text-white">Stack Groups</div>
          <div className="text-slate-400 max-w-md">
            Organize and manage stacks by groups for better organization.
            Create logical groupings based on environments, teams, or applications.
          </div>
          <div className="text-sm text-slate-500">
            Coming soon...
          </div>
        </div>
      </div>
    </div>
  );
}