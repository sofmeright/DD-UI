// ui/src/components/ContainerHealthBadge.tsx
import { useState, useEffect, useCallback } from "react";
import { RefreshCw, Loader2, Heart, HeartOff, AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";
import { handle401 } from "@/utils/auth";
import { 
  Tooltip, 
  TooltipContent, 
  TooltipProvider, 
  TooltipTrigger 
} from "@/components/ui/tooltip";
import { ApiContainer } from "@/types";
import { debugLog } from "@/utils/logging";

interface ContainerHealthBadgeProps {
  container: ApiContainer;
  hostName: string;
  autoRefresh?: boolean;
  refreshInterval?: number;
  onStateChange?: (newState: string, newStatus: string, newHealth?: string) => void;
}

export default function ContainerHealthBadge({ 
  container: initialContainer, 
  hostName,
  autoRefresh = false,
  refreshInterval = 5000,
  onStateChange
}: ContainerHealthBadgeProps) {
  const [container, setContainer] = useState(initialContainer);
  const [loading, setLoading] = useState(false);
  const [lastRefresh, setLastRefresh] = useState(Date.now());
  const [autoRefreshEnabled, setAutoRefreshEnabled] = useState(autoRefresh);
  
  const fetchContainerStatus = useCallback(async () => {
    if (loading) return;
    
    setLoading(true);
    try {
      // Fetch single container status
      const response = await fetch(
        `/api/containers/hosts/${encodeURIComponent(hostName)}/${encodeURIComponent(container.name)}`,
        { credentials: "include" }
      );
      
      if (response.status === 401) {
        handle401();
        return;
      }
      
      if (!response.ok) {
        throw new Error(`Failed to fetch container status: ${response.statusText}`);
      }
      
      const data = await response.json();
      if (data.container) {
        const newContainer = data.container as ApiContainer;
        setContainer(newContainer);
        setLastRefresh(Date.now());
        
        // Notify parent of state change
        if (onStateChange && (
          newContainer.state !== container.state || 
          newContainer.status !== container.status ||
          newContainer.health !== container.health
        )) {
          onStateChange(newContainer.state, newContainer.status, newContainer.health);
        }
        
        debugLog(`[ContainerHealthBadge] Updated ${container.name}: state=${newContainer.state}, health=${newContainer.health}`);
      }
    } catch (error) {
      console.error("Failed to fetch container status:", error);
    } finally {
      setLoading(false);
    }
  }, [container.name, container.state, container.status, container.health, hostName, loading, onStateChange]);
  
  // Auto-refresh effect
  useEffect(() => {
    if (!autoRefreshEnabled) return;
    
    const interval = setInterval(() => {
      fetchContainerStatus();
    }, refreshInterval);
    
    return () => clearInterval(interval);
  }, [autoRefreshEnabled, refreshInterval, fetchContainerStatus]);
  
  // Determine health status and colors
  const getHealthInfo = () => {
    const s = (container.state || "").toLowerCase();
    const st = (container.status || "").toLowerCase();
    const h = (container.health || "").toLowerCase();
    
    let color = "text-slate-400";
    let bgColor = "bg-slate-900/40";
    let borderColor = "border-slate-700/60";
    let icon = <Heart className="h-3.5 w-3.5" />;
    let text = "unknown";
    let animate = false;
    
    if (h === "healthy") {
      color = "text-emerald-400";
      bgColor = "bg-emerald-900/40";
      borderColor = "border-emerald-700/60";
      icon = <Heart className="h-3.5 w-3.5" />;
      text = "healthy";
    } else if (h === "unhealthy") {
      color = "text-rose-400";
      bgColor = "bg-rose-900/40";
      borderColor = "border-rose-700/60";
      icon = <HeartOff className="h-3.5 w-3.5" />;
      text = "unhealthy";
    } else if (h === "starting" || st.includes("starting")) {
      color = "text-amber-400";
      bgColor = "bg-amber-900/40";
      borderColor = "border-amber-700/60";
      icon = <AlertTriangle className="h-3.5 w-3.5" />;
      text = "starting";
      animate = true;
    } else if (st.includes("up") || s.includes("running")) {
      color = "text-emerald-400";
      bgColor = "bg-emerald-900/40";
      borderColor = "border-emerald-700/60";
      icon = <Heart className="h-3.5 w-3.5" />;
      text = "running";
    } else if (s.includes("restarting")) {
      color = "text-amber-400";
      bgColor = "bg-amber-900/40";
      borderColor = "border-amber-700/60";
      icon = <RefreshCw className="h-3.5 w-3.5" />;
      text = "restarting";
      animate = true;
    } else if (s.includes("paused")) {
      color = "text-sky-400";
      bgColor = "bg-sky-900/40";
      borderColor = "border-sky-700/60";
      icon = <AlertTriangle className="h-3.5 w-3.5" />;
      text = "paused";
    } else if (st.includes("exited") || s.includes("exited")) {
      color = "text-rose-400";
      bgColor = "bg-rose-900/40";
      borderColor = "border-rose-700/60";
      icon = <HeartOff className="h-3.5 w-3.5" />;
      
      // Extract exit code if available
      const exitMatch = st.match(/exited\s*\((\d+)\)/i);
      if (exitMatch && exitMatch[1]) {
        const code = exitMatch[1];
        text = code === "0" ? "stopped" : `error (${code})`;
      } else {
        text = "stopped";
      }
    } else if (s.includes("dead")) {
      color = "text-rose-400";
      bgColor = "bg-rose-900/40";
      borderColor = "border-rose-700/60";
      icon = <HeartOff className="h-3.5 w-3.5" />;
      text = "dead";
    } else if (s.includes("removing")) {
      color = "text-orange-400";
      bgColor = "bg-orange-900/40";
      borderColor = "border-orange-700/60";
      icon = <Loader2 className="h-3.5 w-3.5 animate-spin" />;
      text = "removing";
      animate = true;
    }
    
    return { color, bgColor, borderColor, icon, text, animate };
  };
  
  const { color, bgColor, borderColor, icon, text, animate } = getHealthInfo();
  
  return (
    <div className="inline-flex items-center gap-1.5">
      {/* Health Badge */}
      <div className={`
        inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium
        border ${borderColor} ${bgColor} ${color}
        ${animate ? 'animate-pulse' : ''}
      `}>
        {loading ? (
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
        ) : (
          icon
        )}
        <span>{text}</span>
      </div>
      
      {/* Refresh Button */}
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              onClick={fetchContainerStatus}
              disabled={loading}
              variant="ghost"
              size="icon"
              className={`
                h-6 w-6 rounded-full
                ${loading ? 'cursor-not-allowed opacity-50' : 'hover:bg-slate-800'}
                ${autoRefreshEnabled ? 'text-emerald-400' : 'text-slate-400'}
              `}
            >
              {loading ? (
                <Loader2 className="h-3 w-3 animate-spin" />
              ) : (
                <RefreshCw className={`h-3 w-3 ${autoRefreshEnabled ? 'animate-spin-slow' : ''}`} />
              )}
            </Button>
          </TooltipTrigger>
          <TooltipContent side="top">
            <div className="text-xs">
              <div>Click to refresh</div>
              <div className="text-slate-400">
                {autoRefreshEnabled ? 'Auto-refresh enabled' : 'Last: ' + new Date(lastRefresh).toLocaleTimeString()}
              </div>
            </div>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
      
      {/* Auto-refresh Toggle */}
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              onClick={() => setAutoRefreshEnabled(!autoRefreshEnabled)}
              variant="ghost"
              size="icon"
              className="h-6 w-6 rounded-full hover:bg-slate-800"
            >
              <div className={`
                h-2 w-2 rounded-full transition-colors
                ${autoRefreshEnabled ? 'bg-emerald-400 animate-pulse' : 'bg-slate-600'}
              `} />
            </Button>
          </TooltipTrigger>
          <TooltipContent side="top">
            <div className="text-xs">
              {autoRefreshEnabled ? 'Disable' : 'Enable'} auto-refresh
            </div>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>
    </div>
  );
}