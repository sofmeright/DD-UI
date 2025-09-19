// ui/src/views/HostsStacksView.tsx
// ui/src/views/HostsStacksView.tsx
import React, { useEffect, useState } from "react";
import { handle401 } from "@/utils/auth";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { handle401 } from "@/utils/auth";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { handle401 } from "@/utils/auth";
import { Switch } from "@/components/ui/switch";
import EnhancedHostPicker, { PickerOption } from "@/components/EnhancedHostPicker";
import { handle401 } from "@/utils/auth";
import GitSyncToggle from "@/components/GitSyncToggle";
import DevOpsToggle from "@/components/DevOpsToggle";
import {
  ChevronRight,
  ChevronUp,
  ChevronDown,
  FileText,
  Activity,
  Search,
  Pause,
  Play,
  PlayCircle,
  Square,
  RefreshCw,
  RotateCw,
  Terminal,
  Trash2,
  ZapOff,
  Eye,
  Loader2,
  Boxes,
  Layers,
  AlertTriangle,
  XCircle,
  Server,
  Users,
} from "lucide-react";
import StatePill from "@/components/StatePill";
import { handle401 } from "@/utils/auth";
import DriftBadge from "@/components/DriftBadge";
import ActionBtn from "@/components/ActionBtn";
import { handle401 } from "@/utils/auth";
import LiveLogsModal from "@/components/LiveLogsModal";
import ConsoleModal from "@/components/ConsoleModal";
import { handle401 } from "@/utils/auth";
import SearchBar from "@/components/SearchBar";
import PortLinks from "@/components/PortLinks";
import { handle401 } from "@/utils/auth";
import NewStackDialog from "@/components/NewStackDialog";
import { ApiContainer, Host, IacService, IacStack, MergedRow, MergedStack } from "@/types";
import { formatDT, formatPortsLines } from "@/utils/format";
import { handle401 } from "@/utils/auth";
import { debugLog, warnLog } from "@/utils/logging";
import { computeHostMetrics } from "@/utils/metrics";
import { handle401 } from "@/utils/auth";

// Debounce helper to prevent excessive API calls
function useDebounce<T extends (...args: any[]) => any>(func: T, delay: number): T {
  const [timeoutId, setTimeoutId] = useState<NodeJS.Timeout | null>(null);
  
  return ((...args: Parameters<T>) => {
    if (timeoutId) clearTimeout(timeoutId);
    
    const id = setTimeout(() => {
      func(...args);
    }, delay);
    
    setTimeoutId(id);
  }) as T;
}

function sanitizeLabel(s: string): string {
  // match compose label semantics: lowercase, spaces -> _, only [a-z0-9_-]
  const lowered = (s || "").trim().toLowerCase().replaceAll(" ", "_");
  const stripped = lowered.replace(/[^a-z0-9_-]/g, "_");
  return stripped.replace(/^[-_]+|[-_]+$/g, "") || "default";
}

function isEncryptedOrTemplated(v?: string | null) {
  if (!v) return false;
  return v.startsWith("ENC[") || v.includes("${");
}

function guessServiceFromContainerName(containerName: string, projectLabel: string): string | undefined {
  // strip trailing -N or _N
  let base = containerName.replace(/[-_]\d+$/, "");
  // prefer exact compose_project if available; otherwise use provided projectLabel
  // pattern 1: <project>-<service>
  if (base.startsWith(projectLabel + "-")) return base.slice(projectLabel.length + 1);
  // pattern 2: <project>_<service>
  if (base.startsWith(projectLabel + "_")) return base.slice(projectLabel.length + 1);
  return undefined;
}

export default function HostStacksView({
  host,
  hosts,
  onSync,
  onOpenStack,
  onHostChange,
}: {
  host: Host;
  hosts: Host[];
  onSync: () => void;
  onOpenStack: (stackName: string) => void;
  onHostChange: (hostName: string) => void;
}) {
  // Extract unique groups from all hosts
  const allGroups = React.useMemo(() => {
    const groupSet = new Set<string>();
    hosts.forEach(h => {
      if (h.groups) {
        h.groups.forEach(g => groupSet.add(g));
      }
    });
    return Array.from(groupSet).sort();
  }, [hosts]);
  
  // Extract unique tags from all hosts
  const allTags = React.useMemo(() => {
    const tagSet = new Set<string>();
    hosts.forEach(h => {
      if (h.tags) {
        h.tags.forEach(t => tagSet.add(t));
      }
    });
    return Array.from(tagSet).sort();
  }, [hosts]);
  debugLog('[DD-UI] HostStacksView component mounted for host:', host.name);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [stacks, setStacks] = useState<MergedStack[]>([]);
  const [hostQuery, setHostQuery] = useState("");
  const [deletingStacks, setDeletingStacks] = useState<Set<number>>(new Set());
  const [hostMetrics, setHostMetrics] = useState<{ stacks: number; containers: number; drift: number; errors: number }>({ stacks: 0, containers: 0, drift: 0, errors: 0 });
  const [showNewStackDialog, setShowNewStackDialog] = useState(false);
  // Restore host memory feature - use same key as other views
  const getInitialHost = (): string => {
    // First check localStorage for synchronization from other views
    const savedHost = localStorage.getItem('dd_ui_selected_host');
    if (savedHost && hosts.some(h => h.name === savedHost)) {
      // Sync URL if needed
      if (savedHost !== host.name) {
        requestAnimationFrame(() => onHostChange(savedHost));
      }
      return savedHost;
    }
    // Fall back to the passed host prop
    return host.name || '';
  };
  
  // Independent state for host and group selection
  const [selectedHost, setSelectedHost] = useState<string>(getInitialHost());
  const [selectedGroup, setSelectedGroup] = useState<string>(''); // Empty means no group filter
  const [activeSelector, setActiveSelector] = useState<'host' | 'group'>('host'); // Track which selector is active
  
  // Derive viewMode from the combination of selectedHost, selectedGroup, and activeSelector
  const viewMode = React.useMemo((): PickerOption => {
    // Group selector is active and has a selection
    if (activeSelector === 'group' && selectedGroup) {
      return { type: 'group', value: `group:${selectedGroup}`, label: selectedGroup };
    }
    // Host selector is active
    if (activeSelector === 'host') {
      if (selectedHost) {
        return { type: 'host', value: `host:${selectedHost}`, label: selectedHost };
      } else {
        // "All Hosts" is selected
        return { type: 'all', value: 'all', label: 'All' };
      }
    }
    // Group selector is active but "All Groups" is selected - this shouldn't show anything special
    // Just fall back to showing all hosts
    return { type: 'all', value: 'all', label: 'All' };
  }, [selectedHost, selectedGroup, activeSelector]);
  
  // Sync selectedHost with localStorage when it changes
  useEffect(() => {
    if (selectedHost) {
      localStorage.setItem('dd_ui_selected_host', selectedHost);
      // Only navigate if the URL doesn't match
      if (selectedHost !== host.name) {
        onHostChange(selectedHost);
      }
    } else {
      // Don't remove from localStorage when selecting "All" - let user's last specific host persist
    }
  }, [selectedHost, host.name, onHostChange]);
  
  // Listen for localStorage changes from other views
  useEffect(() => {
    const checkStorage = () => {
      const savedHost = localStorage.getItem('dd_ui_selected_host');
      if (savedHost && hosts.some(h => h.name === savedHost)) {
        // Only sync if we're not actively using the host selector for "All Hosts"
        // and we're not in group mode
        if (savedHost !== selectedHost && activeSelector !== 'group' && selectedHost !== '') {
          // Don't override if user explicitly selected "All Hosts" (empty string)
          setSelectedHost(savedHost);
        }
      }
    };
    
    // Check on mount and when returning to this view
    checkStorage();
    
    // Listen for storage events (from other tabs)
    window.addEventListener('storage', checkStorage);
    
    // Also check periodically when the tab is visible
    // BUT only if we haven't explicitly selected "All Hosts"
    const intervalId = setInterval(() => {
      if (document.visibilityState === 'visible' && selectedHost !== '') {
        checkStorage();
      }
    }, 1000);
    
    return () => {
      window.removeEventListener('storage', checkStorage);
      clearInterval(intervalId);
    };
  }, [hosts, selectedHost, activeSelector]);
  const [strictScope, setStrictScope] = useState<boolean>(false); // Show all stacks by default
  const [selectedTags, setSelectedTags] = useState<string[]>([]);
  
  // Sorting and filtering states
  const [sortBy, setSortBy] = useState<'host' | 'name' | 'created' | 'modified' | 'owner'>('name');
  const [sortDirection, setSortDirection] = useState<'asc' | 'desc'>('asc');
  const [filterState, setFilterState] = useState<string>(''); // empty means all
  const [filterOwner, setFilterOwner] = useState<string>(''); // empty means all
  const [filterAllowedUsers, setFilterAllowedUsers] = useState<string>(''); // empty means all
  
  // Performance optimization: debounced sync to prevent excessive API calls
  const debouncedSync = useDebounce(onSync, 300);
  
  // Performance optimizations
  const [lastSyncTime, setLastSyncTime] = useState(0);
  const [pendingContainerUpdates, setPendingContainerUpdates] = useState<Set<string>>(new Set());

  // Container action loading states
  const [actionLoading, setActionLoading] = useState<Set<string>>(new Set());
  const [notification, setNotification] = useState<{
    type: 'success' | 'error';
    message: string;
  } | null>(null);

  // Live logs & console wiring
  const [streamLogs, setStreamLogs] = useState<{ ctr: string; host?: string } | null>(null);
  const [consoleTarget, setConsoleTarget] = useState<{ ctr: string; cmd?: string; host?: string } | null>(null);

  // Lightweight info modal
  const [infoModal, setInfoModal] = useState<{ title: string; text: string } | null>(null);

  // Auto-dismiss notifications
  useEffect(() => {
    if (notification) {
      const timeout = setTimeout(() => setNotification(null), 4000);
      return () => clearTimeout(timeout);
    }
  }, [notification]);

  function matchRow(r: MergedRow, q: string) {
    if (!q) return true;
    const hay = [r.name, r.state, r.stack, r.imageRun, r.imageIac, r.ip, r.portsText, r.owner] .filter(Boolean) .join(" ") .toLowerCase();
    return hay.includes(q.toLowerCase());
  }

  // Calculate metrics for currently visible/filtered stacks
  function calculateFilteredMetrics() {
    const filteredStacks = stacks.filter((s) => {
      // Apply strict scope filter
      if (strictScope) {
        // For host filter, only show host-scoped stacks for that host
        if (selectedHost && (s.scopeKind !== 'host' || s.scopeName !== selectedHost)) {
          return false;
        }
        // For group filter, only show group-scoped stacks for that group
        if (selectedGroup && (s.scopeKind !== 'group' || s.scopeName !== selectedGroup)) {
          return false;
        }
      }
      
      // Apply state filter (only if not 'All States')
      if (filterState && filterState !== '') {
        const hasMatchingState = s.rows.some(r => {
          const state = (r.state || '').toLowerCase();
          if (filterState === 'running') return state.includes('running') || state.includes('up') || state.includes('healthy');
          if (filterState === 'stopped') return state.includes('exited') || state.includes('stopped') || state.includes('down') || state === 'created';
          if (filterState === 'paused') return state.includes('paused');
          if (filterState === 'missing') return state.includes('missing') || state === 'missing';
          return false;
        });
        if (!hasMatchingState) return false;
      }
      
      // Apply owner filter
      if (filterOwner) {
        const hasMatchingOwner = s.rows.some(r => 
          (r.owner || '').toLowerCase().includes(filterOwner.toLowerCase())
        );
        if (!hasMatchingOwner) return false;
      }
      
      // Apply allowed users filter (placeholder for now)
      if (filterAllowedUsers) {
        // This would need implementation based on host/group metadata
        // For now, don't filter by this
      }
      
      // Hide stack cards if search is active and no containers match
      if (!hostQuery.trim()) return true;
      const matchingRows = s.rows.filter((r) => matchRow(r, hostQuery));
      return matchingRows.length > 0;
    });

    return {
      stacks: filteredStacks.length,
      containers: filteredStacks.reduce((sum, s) => {
        // Count only visible containers for each stack
        const visibleContainers = hostQuery.trim() 
          ? s.rows.filter((r) => matchRow(r, hostQuery))
          : s.rows;
        return sum + visibleContainers.length;
      }, 0),
      drift: filteredStacks.filter(s => s.drift === 'drift').length,
      errors: filteredStacks.reduce((sum, s) => {
        const visibleContainers = hostQuery.trim() 
          ? s.rows.filter((r) => matchRow(r, hostQuery))
          : s.rows;
        return sum + visibleContainers.filter(r => r.state === 'exited' || r.state === 'dead').length;
      }, 0),
    };
  }

  async function doCtrAction(ctr: string, action: string, targetHost?: string) {
    // Use provided targetHost or fall back to host.name
    const hostToUse = targetHost || host.name;
    
    // Include host in the action key to differentiate containers with same name on different hosts
    const actionKey = `${hostToUse}-${ctr}-${action}`;
    
    // Set loading state
    setActionLoading(prev => new Set(prev).add(actionKey));
    
    try {
      const response = await fetch(
        `/api/containers/hosts/${encodeURIComponent(hostToUse)}/${encodeURIComponent(ctr)}/action`,
        {
          method: "POST",
          credentials: "include",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ action }),
        }
      );
      
      if (response.status === 401) {
        handle401();
        return;
      }
      
      if (!response.ok) {
        const errorText = await response.text().catch(() => "Action failed");
        throw new Error(errorText);
      }

      // Show success notification
      setNotification({
        type: 'success',
        message: `Container ${ctr} ${action}ed successfully`
      });

      // Smart update: Only refresh container data, not full page sync
      await updateContainerStatus(ctr, action, hostToUse);
      
    } catch (e) {
      // Show error notification
      setNotification({
        type: 'error',
        message: `Failed to ${action} ${ctr}: ${e instanceof Error ? e.message : String(e)}`
      });
    } finally {
      // Clear loading state
      setActionLoading(prev => {
        const next = new Set(prev);
        next.delete(actionKey);
        return next;
      });
    }
  }

  function openLiveLogs(ctr: string, targetHost?: string) {
    setStreamLogs({ ctr, host: targetHost });
  }
  
  // Helper function to create basic stacks from containers only (fast initial render)
  function createBasicStacksFromContainers(runtime: ApiContainer[], hostName?: string): MergedStack[] {
    const rtByStack = new Map<string, ApiContainer[]>();
    
    for (const c of runtime) {
      const label = (c.compose_project || c.stack || "(none)").trim() || "(none)";
      if (!rtByStack.has(label)) rtByStack.set(label, []);
      rtByStack.get(label)!.push(c);
    }
    
    return Array.from(rtByStack.entries()).map(([stackName, containers]) => {
      const rows: MergedRow[] = containers.map(c => {
        const portsLines = formatPortsLines((c as any).ports);
        const portsText = portsLines.join("\n");
        return {
          name: c.name,
          state: c.state,
          status: c.status,
          stack: stackName,
          imageRun: c.image,
          imageIac: undefined,
          created: formatDT(c.created_ts),
          modified: formatDT(c.updated_at),
          ip: c.ip_addr,
          portsText,
          ports: (c as any).ports,
          owner: c.owner || "—",
          // Add the actual host for this container so actions work correctly
          actualHost: hostName,
        } as any;
      });
      
      return {
        name: stackName,
        drift: "unknown" as const,
        iacEnabled: false,
        autoDevOps: false,
        pullPolicy: undefined,
        sops: false,
        deployKind: stackName === "(none)" ? "unmanaged" as const : "unmanaged" as const,
        rows,
        iacId: undefined,
        hasIac: false,
        hasContent: false,
      };
    });
  }

  // Enhanced container status update with reliable polling for state transitions
  async function updateContainerStatus(containerName: string, action?: string, targetHost?: string) {
    // Use provided targetHost or fall back to host.name
    const hostToUse = targetHost || host.name;
    
    // Use host+container as the key to prevent duplicate updates for the same container ON THE SAME HOST
    const updateKey = `${hostToUse}-${containerName}`;
    
    // Prevent duplicate updates for the same container on the same host
    if (pendingContainerUpdates.has(updateKey)) {
      debugLog(`[DD-UI] Skipping duplicate update for ${containerName} on ${hostToUse}`);
      return;
    }
    
    setPendingContainerUpdates(prev => new Set(prev).add(updateKey));
    
    try {
      const maxAttempts = 20;
      let attempts = 0;
      let lastState = '';
      
      while (attempts < maxAttempts) {
        attempts++;
        
        // Progressive delay: optimized for different action types
        let delay: number;
        if (action === 'pause' || action === 'unpause') {
          // Pause/unpause are near-instantaneous - check quickly
          delay = attempts <= 3 ? 150 : attempts <= 8 ? 300 : 800;
        } else {
          // Other actions may need more time
          delay = attempts <= 5 ? 400 : attempts <= 10 ? 1000 : 2000;
        }
        await new Promise(resolve => setTimeout(resolve, delay));
        
        // Fetch container data
        const response = await fetch(`/api/containers/hosts/${encodeURIComponent(hostToUse)}`, { 
          credentials: "include" 
        });
        
        if (response.status === 401) {
          handle401();
          return;
        }
        
        if (!response.ok) {
          debugLog(`[DD-UI] Failed to fetch container data for ${containerName} (attempt ${attempts})`);
          continue;
        }
        
        const data = await response.json();
        const updatedContainers: ApiContainer[] = data.containers || [];
        const updatedContainer = updatedContainers.find(c => c.name === containerName);
        
        if (!updatedContainer) {
          debugLog(`[DD-UI] Container ${containerName} not found (attempt ${attempts})`);
          continue;
        }
        
        const currentState = updatedContainer.state;
        const currentStatus = updatedContainer.status || '';
        
        debugLog(`[DD-UI] Container ${containerName} polling (${attempts}/${maxAttempts}) for action '${action || 'unknown'}': ${currentState} | ${currentStatus}`);
        
        // Always update the UI with current data - ONLY for the specific container on the specific host
        setStacks(prevStacks => 
          prevStacks.map(stack => ({ ...stack,
            rows: stack.rows.map(row => {
              // MUST check both container name AND host to avoid updating wrong containers
              if (row.name === containerName && (row as any).actualHost === hostToUse) {
                const portsLines = formatPortsLines((updatedContainer as any).ports);
                const portsText = portsLines.join("\n");
                return { ...row,
                  state: currentState,
                  status: currentStatus,
                  imageRun: updatedContainer.image,
                  created: formatDT(updatedContainer.created_ts),
                  modified: formatDT(updatedContainer.updated_at),
                  ip: updatedContainer.ip_addr,
                  portsText,
                  ports: (updatedContainer as any).ports,
                  owner: updatedContainer.owner || "—"
                };
              }
              return row;
            })
          }))
        );
        
        // Stop polling conditions:
        
        // 1. Final/stable states - stop immediately
        if (currentState === 'running' || currentState === 'exited' || 
            currentState === 'dead' || currentState === 'paused') {
          debugLog(`[DD-UI] Container ${containerName} reached stable state: ${currentState}`);
          break;
        }
        
        // 2. No state change for 3+ consecutive checks - probably stable
        if (currentState === lastState && attempts >= 8) {
          debugLog(`[DD-UI] Container ${containerName} state unchanged for multiple checks: ${currentState}`);
          break;
        }
        
        // 3. Clear transitioning states - keep polling
        if (currentState === 'restarting' || currentState === 'removing' || 
            currentStatus.toLowerCase().includes('starting')) {
          debugLog(`[DD-UI] Container ${containerName} still transitioning, continue polling...`);
        }
        
        lastState = currentState;
      }
      
      if (attempts >= maxAttempts) {
        debugLog(`[DD-UI] Container ${containerName} polling completed after max attempts`);
      }
      
    } catch (error) {
      debugLog(`[DD-UI] Container status update failed for ${containerName} on ${hostToUse}:`, error);
    } finally {
      // Clean up immediately - the smart polling system will handle regular updates
      setPendingContainerUpdates(prev => {
        const next = new Set(prev);
        next.delete(updateKey); // Use the same host+container key we used to track it
        return next;
      });
    }
  }

  // Data loading happens automatically via the useEffect when dependencies change
  
  useEffect(() => {
    debugLog('[DD-UI] HostStacksView useEffect starting, viewMode:', viewMode);
    let cancel = false;
    
    // For group view, we need to fetch data differently
    const isGroupView = viewMode.type === 'group';
    const isAllView = viewMode.type === 'all';
    const scopeName = viewMode.type === 'group' ? viewMode.value.replace('group:', '') : host.name;
    
    // Register view for polling boost (only for single host view)
    const registerView = async () => {
      if (viewMode.type !== 'host') return;
      try {
        const r = await fetch(`/api/view/hosts/${encodeURIComponent(host.name)}/start`, {
          method: 'POST',
          credentials: 'include'
        });
        if (r.status === 401) {
          handle401();
          return;
        }
        debugLog('[DD-UI] Registered view boost for host:', host.name);
      } catch (e) {
        debugLog('[DD-UI] Failed to register view boost:', e);
      }
    };
    
    // Unregister view for polling boost
    const unregisterView = async () => {
      if (viewMode.type !== 'host') return;
      try {
        const r = await fetch(`/api/view/hosts/${encodeURIComponent(host.name)}/end`, {
          method: 'POST',
          credentials: 'include'
        });
        if (r.status === 401) {
          handle401();
          return;
        }
        debugLog('[DD-UI] Unregistered view boost for host:', host.name);
      } catch (e) {
        debugLog('[DD-UI] Failed to unregister view boost:', e);
      }
    };
    
    registerView(); // Start view boost if needed
    
    (async () => {
      setLoading(true);
      setErr(null);
      
      // Declare variables at top level for use in common code
      let merged: MergedStack[] = [];
      let runtime: ApiContainer[] = [];
      let iacStacks: IacStack[] = [];
      const enhancedByName = new Map<
        string,
        {
          drift_detected: boolean;
          drift_reason?: string;
          effective_auto_devops?: boolean;
          containers?: Array<{ name: string; config_hash?: string }>;
          rendered_services?: Array<{ service_name: string; container_name?: string; image?: string }>;
        }
      >();
      
      try {
        // Determine which hosts to fetch based on view mode and filters
        let hostsToFetch: Host[] = [];
        
        if (isAllView) {
          // For All view, get all hosts (filtered by tags if applicable)
          debugLog('[DD-UI] All view - fetching all hosts');
          
          // Build query params for host filtering
          const hostParams = new URLSearchParams();
          if (selectedTags.length > 0) {
            selectedTags.forEach(tag => hostParams.append('tags', tag));
          }
          
          const hostsResponse = await fetch(`/api/hosts?${hostParams.toString()}`, { credentials: 'include' });
          if (hostsResponse.status === 401) {
            window.location.replace("/auth/login");
            return;
          }
          
          if (hostsResponse.ok) {
            const hostsData = await hostsResponse.json();
            const items = Array.isArray(hostsData.items) ? hostsData.items : [];
            hostsToFetch = items.map((h: any) => ({ 
              name: h.name, 
              address: h.addr ?? h.address ?? "", 
              groups: h.groups ?? [] 
            }));
          }
          
          debugLog('[DD-UI] Will fetch data for', hostsToFetch.length, 'hosts');
          
        } else if (isGroupView) {
          // For Group view, get hosts that are members of the group
          debugLog('[DD-UI] Group view - fetching hosts for group:', selectedGroup);
          
          const groupResponse = await fetch(`/api/groups/${encodeURIComponent(selectedGroup)}/hosts`, { 
            credentials: 'include' 
          });
          
          if (groupResponse.status === 401) {
            window.location.replace("/auth/login");
            return;
          }
          
          if (groupResponse.ok) {
            const groupHosts = await groupResponse.json();
            // The response should be an array of host names
            const hostNames = Array.isArray(groupHosts) ? groupHosts : [];
            
            // Now get the full host details for each
            if (hostNames.length > 0) {
              const allHostsResponse = await fetch('/api/hosts', { credentials: 'include' });
              if (allHostsResponse.ok) {
                const allHostsData = await allHostsResponse.json();
                const items = Array.isArray(allHostsData.items) ? allHostsData.items : [];
                
                // Filter to just the hosts in this group
                hostsToFetch = items
                  .filter((h: any) => hostNames.includes(h.name))
                  .map((h: any) => ({ 
                    name: h.name, 
                    address: h.addr ?? h.address ?? "", 
                    groups: h.groups ?? [] 
                  }));
              }
            }
          }
          
          debugLog('[DD-UI] Will fetch data for', hostsToFetch.length, 'group member hosts');
          
        } else {
          // Single host view - just fetch that host
          const targetHost = selectedHost || host.name;
          hostsToFetch = [host];
          debugLog('[DD-UI] Single host view for:', targetHost);
        }
        
        // Now fetch data symmetrically for all determined hosts
        if (hostsToFetch.length > 0) {
          debugLog('[DD-UI] Starting parallel data fetch for', hostsToFetch.length, 'hosts');
          
          // Fetch container and IaC data for all hosts in parallel
          const hostDataPromises = hostsToFetch.map(async (fetchHost) => {
            try {
              const [containerResponse, iacResponse] = await Promise.all([
                fetch(`/api/containers/hosts/${encodeURIComponent(fetchHost.name)}`, { credentials: 'include' }),
                fetch(`/api/iac/scopes/${encodeURIComponent(fetchHost.name)}`, { credentials: 'include' })
              ]);
              
              if (containerResponse.status === 401 || iacResponse.status === 401) {
                window.location.replace("/auth/login");
                return null;
              }
              
              const containerData = containerResponse.ok ? await containerResponse.json() : { containers: [] };
              const iacData = iacResponse.ok ? await iacResponse.json() : { stacks: [] };
              
              return {
                host: fetchHost,
                containers: containerData.containers || [],
                iacStacks: iacData.stacks || []
              };
            } catch (e) {
              debugLog('[DD-UI] Error fetching data for host', fetchHost.name, ':', e);
              return {
                host: fetchHost,
                containers: [],
                iacStacks: []
              };
            }
          });
          
          const allHostData = await Promise.all(hostDataPromises);
          const validHostData = allHostData.filter(d => d !== null);
          
          debugLog('[DD-UI] Received data for', validHostData.length, 'hosts');
          
          // Now merge all the data into our unified view
          const allStacks: MergedStack[] = [];
          
          for (const hostData of validHostData) {
            if (!hostData) continue;
            
            const { host: dataHost, containers, iacStacks: hostIacStacks } = hostData;
            
            // Process this host's data using existing merge logic
            const hostRuntime = containers as ApiContainer[];
            const hostIac = hostIacStacks as IacStack[];
            
            // Build enhanced map for this host
            const hostEnhanced = new Map<string, any>();
            for (const s of hostIac) {
              if (s.name) {
                debugLog(`[DD-UI] IaC stack ${s.name} drift data:`, {
                  drift_detected: s.drift_detected,
                  drift_reason: s.drift_reason,
                  stack_data: s
                });
                hostEnhanced.set(s.name, {
                  drift_detected: s.drift_detected || false,
                  drift_reason: s.drift_reason || "",
                  effective_auto_devops: s.effective_auto_devops || false,
                  containers: s.containers || [],
                  rendered_services: s.rendered_services || []
                });
              }
            }
            
            // Merge containers and IaC for this host
            const hostMerged = createBasicStacksFromContainers(hostRuntime, dataHost.name);
            
            // Set default scope information for ALL stacks from this host
            for (const stack of hostMerged) {
              // Default scope is the host we're currently processing
              stack.scopeKind = 'host';
              stack.scopeName = dataHost.name;
              // Track the actual physical host for Docker operations
              stack.actualHost = dataHost.name;
            }
            
            // Enhance with IaC data
            for (const stack of hostMerged) {
              const iac = hostIac.find(i => i.name === stack.name);
              if (iac) {
                stack.iacId = iac.id;
                stack.hasIac = true;
                stack.iacEnabled = iac.iac_enabled || false;
                stack.autoDevOps = iac.auto_devops || false;
                stack.pullPolicy = iac.pull_policy;
                stack.sops = iac.sops_status === 'all';
                stack.deployKind = iac.deploy_kind || 'compose';
                stack.hasContent = iac.has_content || false;
                
                // Add scope information for All/Group views
                stack.scopeKind = iac.scope_kind;
                stack.scopeName = iac.scope_name || dataHost.name;
                // Keep tracking the actual host (doesn't change even for group stacks)
                stack.actualHost = dataHost.name;
                
                // Add drift info
                const enh = hostEnhanced.get(stack.name);
                if (enh) {
                  stack.drift = enh.drift_detected ? 'drift' : 'in_sync';
                  stack.driftReason = enh.drift_reason;
                  debugLog(`[DD-UI] Updated drift for stack ${stack.name}: ${stack.drift}`);
                } else {
                  debugLog(`[DD-UI] No enhanced data found for stack ${stack.name}, keeping drift: ${stack.drift}`);
                }
              }
            }
            
            // Add IaC stacks that have no runtime containers
            for (const iac of hostIac) {
              if (!hostMerged.find(s => s.name === iac.name)) {
                const enh = hostEnhanced.get(iac.name);
                hostMerged.push({
                  name: iac.name,
                  drift: enh?.drift_detected ? 'drift' : 'in_sync',
                  driftReason: enh?.drift_reason,
                  iacEnabled: iac.iac_enabled || false,
                  autoDevOps: iac.auto_devops || false,
                  pullPolicy: iac.pull_policy,
                  sops: iac.sops_status === 'all',
                  deployKind: iac.deploy_kind || 'compose',
                  rows: [],
                  iacId: iac.id,
                  hasIac: true,
                  hasContent: iac.has_content || false,
                  scopeKind: iac.scope_kind,
                  scopeName: iac.scope_name || dataHost.name,
                  actualHost: dataHost.name,
                });
              }
            }
            
            // Add all this host's stacks to our combined list
            allStacks.push(...hostMerged);
          }
          
          // Set the final merged stacks
          merged = allStacks;
          setStacks(merged);
          
          // Calculate metrics
          const metrics = {
            stacks: merged.length,
            containers: merged.reduce((sum, s) => sum + s.rows.length, 0),
            drift: merged.filter(s => s.drift === 'drift').length,
            errors: merged.filter(s => s.rows.some(r => r.state === 'exited' || r.state === 'dead')).length,
          };
          setHostMetrics(metrics);
          
          debugLog('[DD-UI] Metrics calculated:', metrics);
          debugLog('[DD-UI] Processed', merged.length, 'total stacks across all hosts');
          setLoading(false);
          
        } else {
          // No hosts to fetch - show empty state
          setStacks([]);
          setHostMetrics({ stacks: 0, containers: 0, drift: 0, errors: 0 });
          setLoading(false);
        }
        // Unified API has been removed - using individual host fetching above
      } catch (e: any) {
        if (!cancel) setErr(e?.message || "Failed to load host stacks");
      } finally {
        if (!cancel) setLoading(false);
      }
    })();
    
    return () => {
      cancel = true;
      unregisterView(); // End view boost
    };
  }, [host.name, viewMode, selectedTags, strictScope, selectedHost, selectedGroup]);
  
  // No need for complex localStorage checking - host prop already handles it


  // Periodic polling to refresh container states
  useEffect(() => {
    // Only poll if we have loaded initial data
    if (loading || !stacks.length) return;
    
    let pollInterval: NodeJS.Timeout;
    
    const pollContainerData = async () => {
      try {
        // Get all unique actual hosts from the stacks
        const uniqueHosts = new Set<string>();
        stacks.forEach(stack => {
          if (stack.actualHost) {
            uniqueHosts.add(stack.actualHost);
          }
        });
        
        // If no hosts found, fall back to current host
        if (uniqueHosts.size === 0) {
          uniqueHosts.add(host.name);
        }
        
        debugLog('[DD-UI] Polling for container updates from hosts:', Array.from(uniqueHosts));
        
        // Fetch containers from all actual hosts with host tracking
        const containersByHost = new Map<string, ApiContainer[]>();
        for (const hostName of uniqueHosts) {
          try {
            const response = await fetch(`/api/containers/hosts/${encodeURIComponent(hostName)}`, { 
              credentials: "include" 
            });
            
            if (response.status === 401) {
              handle401();
              return;
            }
            
            if (response.ok) {
              const contJson = await response.json();
              const runtime: ApiContainer[] = (contJson.containers || []) as ApiContainer[];
              containersByHost.set(hostName, runtime);
            }
          } catch (err) {
            debugLog(`[DD-UI] Failed to poll host ${hostName}:`, err);
          }
        }
        
        // Update container states in existing stacks
        setStacks(prevStacks => 
          prevStacks.map(stack => ({ ...stack,
            rows: stack.rows.map(row => {
              // CRITICAL: Must check both container name AND actualHost to avoid updating wrong containers
              const rowHost = (row as any).actualHost;
              if (rowHost && containersByHost.has(rowHost)) {
                const hostContainers = containersByHost.get(rowHost) || [];
                const updatedContainer = hostContainers.find(c => c.name === row.name);
                if (updatedContainer) {
                  return { ...row,
                    state: updatedContainer.state || row.state,
                    status: updatedContainer.status || row.status,
                  };
                }
              }
              return row;
            })
          }))
        );
        debugLog('[DD-UI] Container states updated from polling');
      } catch (error) {
        debugLog('[DD-UI] Polling error:', error);
      }
    };
    
    // Start polling every 2 seconds (frontend refresh rate)
    pollInterval = setInterval(pollContainerData, 2000);
    
    return () => {
      if (pollInterval) clearInterval(pollInterval);
    };
  }, [host.name, loading, stacks.length]);

  // --- Name validation helpers (warn-only; Compose will normalize) ---
  function dockerSanitizePreview(s: string): string {
    const lowered = s.trim().toLowerCase().replaceAll(" ", "_");
    return lowered.replace(/[^a-z0-9_-]/g, "_") || "default";
  }
  function hasUnsupportedChars(s: string): boolean {
    return /[^A-Za-z0-9 _-]/.test(s);
  }

  const handleStackCreated = (scopeKind: string, scopeName: string, stackName: string) => {
    // Navigate to the stack editor
    onOpenStack(stackName);
    // The stacks will automatically refresh via the useEffect
    // Close the dialog
    setShowNewStackDialog(false);
  };


  async function deleteStackAt(index: number) {
    const s = stacks[index];
    if (!s.iacId) return;
    if (!confirm(`Delete IaC for stack "${s.name}"? This removes IaC files/metadata but not runtime containers.`))
      return;

    // Add to deleting set to show loading state
    setDeletingStacks(prev => new Set(prev).add(index));
    
    // Optimistically update UI immediately
    const originalStack = { ...s };
    setStacks((prev) =>
      prev.map((row, i) =>
        i === index ? { ...row,
              iacId: undefined,
              hasIac: false,
              iacEnabled: false,
              autoDevOps: false,  // Clear autoDevOps when deleting stack
              pullPolicy: undefined,
              sops: false,
              drift: "unknown" as const,
              hasContent: false,
            } : row
      )
    );

    try {
      const r = await fetch(`/api/iac/scopes/${encodeURIComponent(s.scope_name || host.name)}/stacks/${encodeURIComponent(s.name)}`, { 
        method: "DELETE", 
        credentials: "include" 
      });
      
      if (r.status === 401) {
        handle401();
        return;
      }
      
      if (!r.ok) {
        // Revert optimistic update on failure
        setStacks((prev) =>
          prev.map((row, i) => i === index ? originalStack : row)
        );
        
        // Show error notification (you might want to replace with a toast system)
        const errorMsg = r.status === 404 ? "Stack not found" : `Failed to delete: ${r.status} ${r.statusText}`;
        console.error("Stack deletion failed:", errorMsg);
        alert(errorMsg);
      }
    } catch (error) {
      // Revert optimistic update on network error
      setStacks((prev) =>
        prev.map((row, i) => i === index ? originalStack : row)
      );
      
      console.error("Network error during stack deletion:", error);
      alert("Network error: Failed to delete stack. Please check your connection and try again.");
    } finally {
      // Remove from deleting set
      setDeletingStacks(prev => {
        const next = new Set(prev);
        next.delete(index);
        return next;
      });
    }
  }

  useEffect(() => {
    if (!infoModal) return;
    const onEsc = (e: KeyboardEvent) => {
      if (e.key === "Escape") setInfoModal(null);
    };
    window.addEventListener("keydown", onEsc);
    return () => window.removeEventListener("keydown", onEsc);
  }, [infoModal]);

  return (
    <div className="space-y-4">
      {/* Streaming Logs Modal */}
      {streamLogs && (
        <LiveLogsModal host={streamLogs.host || host.name} container={streamLogs.ctr} onClose={() => setStreamLogs(null)} />
      )}

      {/* Console Modal */}
      {consoleTarget && (
        <ConsoleModal host={consoleTarget.host || host.name} container={consoleTarget.ctr} onClose={() => setConsoleTarget(null)} />
      )}

      {/* Info Modal */}
      {infoModal && (
        <div
          className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center p-4"
          role="dialog"
          aria-modal="true"
          aria-labelledby="info-title"
          onClick={() => setInfoModal(null)}
        >
          <div
            className="bg-slate-950 border border-slate-800 rounded-xl w-full max-w-3xl p-4"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between mb-2">
              <div className="text-slate-200 font-semibold" id="info-title">
                {infoModal.title}
              </div>
              <Button size="sm" variant="outline" className="border-slate-700" onClick={() => setInfoModal(null)}>
                Close
              </Button>
            </div>
            <pre className="text-xs text-slate-300 bg-slate-900 border border-slate-800 rounded p-3 max-h-[60vh] overflow-auto whitespace-pre-wrap">
              {infoModal.text}
            </pre>
          </div>
        </div>
      )}

      {/* Action Notification */}
      {notification && (
        <div className="fixed top-4 right-4 z-50 max-w-md">
          <div className={`rounded-lg border p-4 shadow-lg backdrop-blur-sm ${
            notification.type === 'success'  ? 'bg-emerald-950/90 border-emerald-800/50 text-emerald-200'  : 'bg-red-950/90 border-red-800/50 text-red-200'
          }`}>
            <div className="flex items-center gap-2">
              {notification.type === 'success' ? (
                <div className="w-2 h-2 bg-emerald-400 rounded-full"></div>
              ) : (
                <div className="w-2 h-2 bg-red-400 rounded-full"></div>
              )}
              <span className="text-sm font-medium">{notification.message}</span>
              <Button
                variant="ghost"
                size="sm"
                className="ml-auto h-6 w-6 p-0 hover:bg-transparent"
                onClick={() => setNotification(null)}
              >
                <span className="sr-only">Close</span>
                ×
              </Button>
            </div>
          </div>
        </div>
      )}

      <div className="flex items-start gap-4 flex-wrap">
        <div className="text-lg font-semibold text-white self-center">Stacks</div>
        
        {/* Host Filter Section */}
        <div className="flex flex-col gap-1 border-l border-slate-700 pl-4">
          <div className="flex items-center gap-2">
            <Server className="h-4 w-4 text-slate-400" />
            <select
              className="bg-slate-800 border border-slate-700 text-slate-200 px-3 py-1 rounded-md text-sm min-w-[150px]"
              value={selectedHost}
              onChange={(e) => {
                setSelectedHost(e.target.value);
                setSelectedGroup(''); // Clear group when selecting host
                setActiveSelector('host'); // Mark host selector as active
                // localStorage sync and viewMode update are handled by useEffects
              }}
            >
              <option value="">All Hosts</option>
              {hosts.map(h => (
                <option key={h.name} value={h.name}>{h.name}</option>
              ))}
            </select>
          </div>
          <div className="flex items-center gap-2 pl-6">
            <Switch
              checked={strictScope}
              onCheckedChange={setStrictScope}
              className="data-[state=checked]:bg-emerald-600 h-4 w-8"
            />
            <label className="text-xs text-slate-400">
              IAC scoped only
            </label>
          </div>
        </div>
        
        {/* Group Filter Section */}
        <div className="flex flex-col gap-1 border-l border-slate-700 pl-4">
          <div className="flex items-center gap-2">
            <Users className="h-4 w-4 text-slate-400" />
            <select
              className="bg-slate-800 border border-slate-700 text-slate-200 px-3 py-1 rounded-md text-sm min-w-[150px]"
              value={selectedGroup}
              onChange={(e) => {
                setSelectedGroup(e.target.value);
                setSelectedHost(''); // Clear host when selecting group - they're mutually exclusive
                setActiveSelector('group'); // Mark group selector as active
                // viewMode is automatically updated via useMemo
              }}
            >
              <option value="">All Groups</option>
              {allGroups.map(g => (
                <option key={g} value={g}>{g}</option>
              ))}
            </select>
          </div>
          <div className="flex items-center gap-2 pl-6">
            <Switch
              checked={strictScope && selectedGroup !== ''}
              disabled={!selectedGroup}
              onCheckedChange={setStrictScope}
              className="data-[state=checked]:bg-emerald-600 h-4 w-8"
            />
            <label className="text-xs text-slate-400">
              IAC scoped only
            </label>
          </div>
        </div>
        
        {/* Tag Filter */}
        {allTags.length > 0 && (
          <div className="flex flex-col gap-1">
            <label className="text-xs text-slate-400">Filter by Tags</label>
            <div className="relative">
              <select
                multiple
                value={selectedTags}
                onChange={(e) => {
                  const selected = Array.from(e.target.selectedOptions, option => option.value);
                  setSelectedTags(selected);
                }}
                className="bg-slate-900 text-slate-200 border border-slate-700 rounded-lg px-3 py-1.5 pr-8 text-sm w-48"
                style={{ minHeight: '36px', maxHeight: '120px' }}
              >
                {allTags.map(tag => (
                  <option key={tag} value={tag}>{tag}</option>
                ))}
              </select>
              {selectedTags.length > 0 && (
                <button
                  onClick={() => {
                    setSelectedTags([]);
                  }}
                  className="absolute right-2 top-2 text-slate-400 hover:text-slate-200"
                >
                  ×
                </button>
              )}
            </div>
            {selectedTags.length > 0 && (
              <div className="flex flex-wrap gap-1 mt-1">
                {selectedTags.map(tag => (
                  <span key={tag} className="px-2 py-0.5 bg-slate-800 text-slate-300 text-xs rounded">
                    {tag}
                  </span>
                ))}
              </div>
            )}
          </div>
        )}
        
        {/* Status metric cards for currently visible stacks */}
        <div className="flex items-center gap-2">
          {(() => {
            const filteredMetrics = calculateFilteredMetrics();
            return (
              <>
                <div className="px-3 py-2 bg-slate-900/60 border border-slate-800 rounded-lg flex items-center gap-2">
                  <Layers className="h-4 w-4 text-slate-400" />
                  <span className="text-sm text-slate-300">{filteredMetrics.stacks}</span>
                </div>
                <div className="px-3 py-2 bg-slate-900/60 border border-slate-800 rounded-lg flex items-center gap-2">
                  <Boxes className="h-4 w-4 text-slate-400" />
                  <span className="text-sm text-slate-300">{filteredMetrics.containers}</span>
                </div>
                <div className="px-3 py-2 bg-slate-900/60 border border-slate-800 rounded-lg flex items-center gap-2">
                  <AlertTriangle className="h-4 w-4 text-amber-400" />
                  <span className="text-sm text-amber-400">{filteredMetrics.drift}</span>
                </div>
                <div className="px-3 py-2 bg-slate-900/60 border border-slate-800 rounded-lg flex items-center gap-2">
                  <XCircle className="h-4 w-4 text-rose-400" />
                  <span className="text-sm text-rose-400">{filteredMetrics.errors}</span>
                </div>
              </>
            );
          })()}
        </div>
        
        {/* Toggles positioned at the end */}
        <div className="ml-auto flex items-center gap-3">
          <DevOpsToggle level="host" hostName={host.name} />
          <GitSyncToggle />
        </div>
      </div>
      
      {/* Sort and Filter Controls */}
      <div className="flex items-center gap-3 mt-3 mb-1">
          {/* Sort By */}
          <div className="flex items-center gap-2">
            <span className="text-xs text-slate-400">Sort By:</span>
            <select
              value={sortBy}
              onChange={(e) => setSortBy(e.target.value as typeof sortBy)}
              className="bg-slate-900 text-slate-200 border border-slate-700 rounded px-2 py-1 text-xs"
            >
              {viewMode.type === 'all' && <option value="host">Host</option>}
              <option value="name">Name</option>
              <option value="created">Created</option>
              <option value="modified">Modified</option>
              <option value="owner">Owner</option>
            </select>
            <button
              onClick={() => setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc')}
              className="bg-slate-800 hover:bg-slate-700 text-slate-200 p-1 rounded transition-colors flex items-center justify-center"
              title={sortDirection === 'asc' ? 'Ascending' : 'Descending'}
            >
              {sortDirection === 'asc' ? <ChevronDown className="h-4 w-4" /> : <ChevronUp className="h-4 w-4" />}
            </button>
          </div>
          
          {/* Filter By */}
          <div className="flex items-center gap-2">
            <span className="text-xs text-slate-400">Filter By:</span>
            
            {/* State Filter */}
            <select
              value={filterState}
              onChange={(e) => setFilterState(e.target.value)}
              className="bg-slate-900 text-slate-200 border border-slate-700 rounded px-2 py-1 text-xs"
            >
              <option value="">All States</option>
              <option value="running">Running</option>
              <option value="stopped">Stopped</option>
              <option value="paused">Paused</option>
              <option value="missing">Missing</option>
            </select>
            
            {/* Owner Filter */}
            <input
              type="text"
              placeholder="Owner..."
              value={filterOwner}
              onChange={(e) => setFilterOwner(e.target.value)}
              className="bg-slate-900 text-slate-200 border border-slate-700 rounded px-2 py-1 text-xs w-24"
            />
            
            {/* Allowed Users Filter */}
            <input
              type="text"
              placeholder="Allowed Users..."
              value={filterAllowedUsers}
              onChange={(e) => setFilterAllowedUsers(e.target.value)}
              className="bg-slate-900 text-slate-200 border border-slate-700 rounded px-2 py-1 text-xs w-32"
            />
            
            {/* Search, Sync, and New Stack moved here */}
            <SearchBar 
              value={hostQuery}
              onChange={setHostQuery}
              placeholder="Search stacks, services, containers..."
              className="w-96"
            />
            <Button onClick={onSync} className="bg-[#310937] hover:bg-[#2a0830] text-white text-xs py-1 px-2" disabled={loading}>
              {loading ? (
                <Loader2 className="h-3 w-3 mr-1 animate-spin" />
              ) : (
                <RefreshCw className="h-3 w-3 mr-1" />
              )} 
              {loading ? "Syncing..." : "Sync"}
            </Button>
            <Button onClick={() => setShowNewStackDialog(true)} variant="outline" className="border-slate-700 text-slate-200 text-xs py-1 px-2">
              New Stack
            </Button>
            
            {(filterState || filterOwner || filterAllowedUsers) && (
              <button
                onClick={() => {
                  setFilterState('');
                  setFilterOwner('');
                  setFilterAllowedUsers('');
                }}
                className="text-slate-400 hover:text-slate-200 text-xs"
              >
                Clear
              </button>
            )}
          </div>
        </div>
      
      <div className="text-xs text-slate-400 mt-1">Tip: click the stack title to open the full compare & editor view.</div>

      {loading && (
        <div className="space-y-3">
          <div className="text-sm px-3 py-2 rounded-lg border border-slate-800 bg-slate-900/60 text-slate-300 flex items-center gap-2">
            <Loader2 className="h-4 w-4 animate-spin" />
            Loading stacks…
            {stacks.length > 0 && <span className="text-slate-400">({stacks.length} loaded)</span>}
          </div>
          {stacks.length === 0 && (
            <div className="space-y-3">
              {loading ? (
                /* Skeleton loading cards */
                [1, 2, 3].map(i => (
                  <Card key={i} className="bg-slate-900/50 border-slate-800 rounded-xl animate-pulse">
                    <CardHeader className="pb-2">
                      <div className="h-6 bg-slate-700 rounded w-32"></div>
                      <div className="flex gap-2 mt-2">
                        <div className="h-4 bg-slate-700 rounded w-16"></div>
                        <div className="h-4 bg-slate-700 rounded w-20"></div>
                      </div>
                  </CardHeader>
                  <CardContent>
                    <div className="h-24 bg-slate-800 rounded"></div>
                  </CardContent>
                </Card>
                ))
              ) : (
                /* No data message */
                <Card className="bg-slate-900/50 border-slate-800 rounded-xl">
                  <CardContent className="py-8 text-center">
                    <div className="text-slate-400">
                      {viewMode.type === 'all' ? 
                        'No stacks found across all hosts' : 
                        `No stacks found for ${viewMode.label}`}
                    </div>
                    {err && (
                      <div className="mt-2 text-sm text-rose-400">
                        Error: {err}
                      </div>
                    )}
                  </CardContent>
                </Card>
              )}
            </div>
          )}
        </div>
      )}
      {err && (
        <div className="text-sm px-3 py-2 rounded-lg border border-rose-800/50 bg-rose-950/50 text-rose-200">
          Error: {err}
        </div>
      )}

      {stacks
        .filter((s) => {
          // Apply strict scope filter
          if (strictScope) {
            // For host filter, only show host-scoped stacks for that host
            if (selectedHost && (s.scopeKind !== 'host' || s.scopeName !== selectedHost)) {
              return false;
            }
            // For group filter, only show group-scoped stacks for that group
            if (selectedGroup && (s.scopeKind !== 'group' || s.scopeName !== selectedGroup)) {
              return false;
            }
          }
          
          // Apply state filter (only if not 'All States')
          if (filterState && filterState !== '') {
            const hasMatchingState = s.rows.some(r => {
              const state = (r.state || '').toLowerCase();
              if (filterState === 'running') return state.includes('running') || state.includes('up') || state.includes('healthy');
              if (filterState === 'stopped') return state.includes('exited') || state.includes('stopped') || state.includes('down') || state === 'created';
              if (filterState === 'paused') return state.includes('paused');
              if (filterState === 'missing') return state.includes('missing') || state === 'missing';
              return false;
            });
            if (!hasMatchingState) return false;
          }
          
          // Apply owner filter
          if (filterOwner) {
            const hasMatchingOwner = s.rows.some(r => 
              (r.owner || '').toLowerCase().includes(filterOwner.toLowerCase())
            );
            if (!hasMatchingOwner) return false;
          }
          
          // Apply allowed users filter (this would need to come from host/group metadata)
          // For now, this is a placeholder as allowed users aren't tracked at stack level
          if (filterAllowedUsers) {
            // TODO: Implement when allowed users data is available at stack level
          }
          
          // Hide stack cards if search is active and no containers match
          if (!hostQuery.trim()) return true;
          const matchingRows = s.rows.filter((r) => matchRow(r, hostQuery));
          return matchingRows.length > 0;
        })
        .sort((a, b) => {
          let compareValue = 0;
          
          switch (sortBy) {
            case 'host':
              compareValue = (a.scopeName || '').localeCompare(b.scopeName || '');
              break;
            case 'name':
              compareValue = a.name.localeCompare(b.name);
              break;
            case 'created':
              // Get earliest created timestamp from containers
              const aCreated = Math.min(...a.rows.map(r => {
                const timestamp = Date.parse(r.created || '0');
                return isNaN(timestamp) ? Infinity : timestamp;
              }));
              const bCreated = Math.min(...b.rows.map(r => {
                const timestamp = Date.parse(r.created || '0');
                return isNaN(timestamp) ? Infinity : timestamp;
              }));
              compareValue = aCreated - bCreated;
              break;
            case 'modified':
              // Get latest modified timestamp from containers (for now use created as proxy)
              const aModified = Math.max(...a.rows.map(r => {
                const timestamp = Date.parse(r.created || '0');
                return isNaN(timestamp) ? 0 : timestamp;
              }));
              const bModified = Math.max(...b.rows.map(r => {
                const timestamp = Date.parse(r.created || '0');
                return isNaN(timestamp) ? 0 : timestamp;
              }));
              compareValue = bModified - aModified; // Most recent first
              break;
            case 'owner':
              // Get most common owner from containers
              const getOwner = (stack: MergedStack) => {
                const owners = stack.rows.map(r => r.owner || '—').filter(o => o !== '—');
                return owners.length > 0 ? owners[0] : '—';
              };
              compareValue = getOwner(a).localeCompare(getOwner(b));
              break;
          }
          
          // Apply sort direction
          return sortDirection === 'asc' ? compareValue : -compareValue;
        })
        .map((s, idx) => (
        <Card key={`${s.actualHost || s.scopeName || host.name}:${s.name}`} className="bg-slate-900/50 border-slate-800 rounded-xl">
          <CardHeader className="py-2 flex flex-row items-center justify-between">
            <div className="space-y-1 flex-1">
              <CardTitle className="text-xl text-white flex items-center gap-3">
                <button className="hover:underline" onClick={() => {
                  // Navigate to the stack's actual host if different from current
                  if (s.scopeName && s.scopeName !== host.name) {
                    onHostChange(s.scopeName);
                    setTimeout(() => onOpenStack(s.name), 100);
                  } else {
                    onOpenStack(s.name);
                  }
                }}>
                  {s.name}
                </button>
                {/* Scope badge for mixed view */}
                {viewMode.type === 'all' && (
                  <Badge 
                    variant="outline" 
                    className={s.scopeKind === 'group' 
                      ? "border-blue-600 bg-blue-900/20 text-blue-300" 
                      : "border-green-600 bg-green-900/20 text-green-300"}
                  >
                    {s.scopeKind === 'group' ? (
                      <><Users className="h-3 w-3 mr-1" />GROUP: {s.scopeName}</>
                    ) : (
                      <><Server className="h-3 w-3 mr-1" />HOST: {s.scopeName}</>
                    )}
                  </Badge>
                )}
                {/* Group badge when viewing a specific group */}
                {viewMode.type === 'group' && (
                  <Badge className="border-blue-600 bg-blue-900/20 text-blue-300" variant="outline">
                    <Users className="h-3 w-3 mr-1" />GROUP STACK
                  </Badge>
                )}
              </CardTitle>
              <div className="flex items-center gap-2">
                <span title={(s as any).driftReason || ""}>{DriftBadge(s.drift)}</span>
                <Badge variant="outline" className="border-slate-700 text-slate-300">
                  {s.deployKind || "unknown"}
                </Badge>
                <Badge variant="outline" className="border-slate-700 text-slate-300">
                  pull: {s.hasIac ? s.pullPolicy || "—" : "—"}
                </Badge>
                {s.hasIac ? (
                  s.sops ? (
                    <Badge className="bg-indigo-900/40 border-indigo-700/40 text-indigo-200">SOPS</Badge>
                  ) : (
                    <Badge variant="outline" className="border-slate-700 text-slate-300">no SOPS</Badge>
                  )
                ) : (
                  <Badge variant="outline" className="border-slate-700 text-slate-300">No IaC</Badge>
                )}
                {s.iacId && (
                  <button 
                    title="Delete IaC for this stack" 
                    onClick={() => deleteStackAt(idx)}
                    disabled={deletingStacks.has(idx)}
                    className={`h-[22px] w-[22px] flex items-center justify-center bg-slate-900/80 border border-slate-700 rounded-full hover:bg-rose-900/40 hover:border-rose-700 transition-colors ${deletingStacks.has(idx) ? "opacity-50" : ""}`}
                  >
                    {deletingStacks.has(idx) ? (
                      <Loader2 className="h-3 w-3 text-rose-400 animate-spin" />
                    ) : (
                      <Trash2 className="h-3 w-3 text-rose-400" />
                    )}
                  </button>
                )}
              </div>
              {/* Show which hosts are running this group stack */}
              {viewMode.type === 'group' && s.runningOnHosts && s.runningOnHosts.length > 0 && (
                <div className="flex items-center gap-2 mt-1">
                  <span className="text-xs text-slate-400">Running on:</span>
                  {s.runningOnHosts.map((hostName: string) => (
                    <Badge key={hostName} variant="outline" className="border-slate-600 text-slate-300 text-xs">
                      <Server className="h-3 w-3 mr-1" />{hostName}
                    </Badge>
                  ))}
                </div>
              )}
            </div>
            <div className="flex items-center">
              <DevOpsToggle
                level="stack"
                hostName={s.scopeName || host}
                stackName={s.name}
                compact={false}
              />
            </div>
          </CardHeader>
          <CardContent className="pt-0">
            <div className="overflow-hidden rounded-lg border border-slate-800">
              <div className="overflow-x-auto">
                <table className="w-full text-sm table-auto">
                  <thead className="bg-slate-900/70 text-slate-300">
                    <tr className="whitespace-nowrap">
                      <th className="px-2 py-2 text-left min-w-[100px] max-w-[150px] w-[13.3%]">Name</th>
                      <th className="px-2 py-2 text-left min-w-[40px] w-[3.85%]">State</th>
                      <th className="px-2 py-2 text-left min-w-[100px] max-w-[140px] w-[13.35%]">Image</th>
                      <th className="px-2 py-2 text-left min-w-[60px] w-[6%]">Ports</th>
                      <th className="px-2 py-2 text-left w-[280px] max-w-[280px]">Actions</th>
                      <th className="px-2 py-2 text-left w-[90px] max-w-[90px] hidden sm:table-cell">Created</th>
                      <th className="px-2 py-2 text-left w-[90px] max-w-[90px] hidden sm:table-cell">Modified</th>
                      <th className="px-2 py-2 text-left w-[100px] max-w-[100px] hidden md:table-cell">IP</th>
                      <th className="px-2 py-2 text-left w-[80px] max-w-[80px] hidden lg:table-cell pr-4">Owner</th>
                    </tr>
                  </thead>
                <tbody>
                  {s.rows .filter((r) => matchRow(r, hostQuery)) .map((r) => {
                      const st = (r.state || "").toLowerCase();
                      const isRunning =
                        st.includes("running") || st.includes("up") || st.includes("healthy") || st.includes("restarting");
                      const isPaused = st.includes("paused");
                      return (
                        <tr key={r.name} className="border-t border-slate-800 hover:bg-slate-900/40 align-top">
                          <td className="px-2 py-1.5 font-medium text-slate-200 max-w-[150px]">
                            <div className="truncate" title={r.name}>
                              {r.name}
                            </div>
                          </td>
                          <td className="px-2 py-1.5 text-slate-300 min-w-[40px] max-w-[80px]">
                            <div className="truncate">
                              <StatePill state={r.state} status={r.status} />
                            </div>
                          </td>
                          <td className="px-2 py-1.5 text-slate-300 max-w-[140px]">
                            <div className="flex items-center gap-2">
                              <div className="truncate" title={r.imageRun || ""}>
                                {r.imageRun || "—"}
                              </div>
                              {r.imageIac && r.state === "missing" && (
                                <>
                                  <ChevronRight className="h-4 w-4 text-slate-500 flex-shrink-0" />
                                  <div className="truncate text-slate-300" title={r.imageIac}>
                                    {r.imageIac}
                                  </div>
                                </>
                              )}
                            </div>
                          </td>
                          <td className="px-2 py-1.5 text-slate-300 align-top min-w-[60px]">
                            <div className="truncate">
                              <PortLinks 
                                ports={r.ports || []} 
                                hostAddress={
                                  // In unified view, use the container's actual host
                                  (r as any).actualHost ? 
                                    (hosts.find(h => h.name === (r as any).actualHost)?.address || (r as any).actualHost) :
                                  // Fall back to stack scope or current host
                                  s.scopeName ? 
                                    (hosts.find(h => h.name === s.scopeName)?.address || s.scopeName) : 
                                    (host.address || host.name)
                                }
                                className="leading-tight"
                              />
                            </div>
                          </td>
                          <td className="px-2 py-1 w-[280px] max-w-[280px]">
                            <div className="inline-flex items-center gap-0 whitespace-nowrap py-1 px-1.5 bg-slate-900/40 rounded-md border border-slate-800/50 w-auto">
                              {r.state === "missing" ? (
                                // For missing services, only show inspect action
                                <>
                                  <ActionBtn title="Inspect" icon={Search} onClick={() => {
                                    // Navigate to the stack's actual host if different from current
                                    if (s.scopeName && s.scopeName !== host.name) {
                                      onHostChange(s.scopeName);
                                      setTimeout(() => onOpenStack(s.name), 100);
                                    } else {
                                      onOpenStack(s.name);
                                    }
                                  }} />
                                  <span className="text-slate-500 text-xs ml-2">Service not running</span>
                                </>
                              ) : (
                                // Container actions organized into logical groups
                                <>
                                  {/* Group 1: Container Lifecycle (stop/start, restart, pause/resume) */}
                                  {!isRunning && !isPaused && (
                                    <ActionBtn 
                                      title="Start" 
                                      icon={Play} 
                                      color="green" 
                                      onClick={() => doCtrAction(r.name, "start", (r as any).actualHost || (r as any).hostName || undefined)}
                                      loading={actionLoading.has(`${(r as any).actualHost || (r as any).hostName || host.name}-${r.name}-start`)}
                                    />
                                  )}
                                  {isRunning && (
                                    <ActionBtn 
                                      title="Stop" 
                                      icon={Square} 
                                      color="yellow" 
                                      onClick={() => doCtrAction(r.name, "stop", (r as any).actualHost || (r as any).hostName || undefined)}
                                      loading={actionLoading.has(`${(r as any).actualHost || (r as any).hostName || host.name}-${r.name}-stop`)}
                                    />
                                  )}
                                  {(isRunning || isPaused) && (
                                    <ActionBtn
                                      title="Restart"
                                      icon={RotateCw}
                                      color="blue"
                                      onClick={() => doCtrAction(r.name, "restart", (r as any).actualHost || (r as any).hostName || undefined)}
                                      loading={actionLoading.has(`${(r as any).actualHost || (r as any).hostName || host.name}-${r.name}-restart`)}
                                    />
                                  )}
                                  {isRunning && !isPaused && (
                                    <ActionBtn 
                                      title="Pause" 
                                      icon={Pause} 
                                      color="yellow" 
                                      onClick={() => doCtrAction(r.name, "pause", (r as any).actualHost || (r as any).hostName || undefined)}
                                      loading={actionLoading.has(`${(r as any).actualHost || (r as any).hostName || host.name}-${r.name}-pause`)}
                                    />
                                  )}
                                  {isPaused && (
                                    <ActionBtn
                                      title="Resume"
                                      icon={PlayCircle}
                                      color="green"
                                      onClick={() => doCtrAction(r.name, "unpause", (r as any).actualHost || (r as any).hostName || undefined)}
                                      loading={actionLoading.has(`${(r as any).actualHost || (r as any).hostName || host.name}-${r.name}-unpause`)}
                                    />
                                  )}

                                  
                                  {/* Group 2: Destructive Actions (kill, remove) */}
                                  <ActionBtn 
                                    title="Kill" 
                                    icon={ZapOff} 
                                    color="red" 
                                    onClick={() => doCtrAction(r.name, "kill", (r as any).actualHost || (r as any).hostName || undefined)}
                                    loading={actionLoading.has(`${(r as any).actualHost || (r as any).hostName || host.name}-${r.name}-kill`)}
                                  />
                                  <ActionBtn 
                                    title="Remove" 
                                    icon={Trash2} 
                                    color="red" 
                                    onClick={() => doCtrAction(r.name, "remove", (r as any).actualHost || (r as any).hostName || undefined)}
                                    loading={actionLoading.has(`${(r as any).actualHost || (r as any).hostName || host.name}-${r.name}-remove`)}
                                  />

                                  <span className="mx-1.5 h-4 w-px bg-slate-700/60" />

                                  {/* Group 3: Information/Monitoring (logs, console, stats, inspect) */}
                                  <ActionBtn title="Logs" icon={FileText} onClick={() => openLiveLogs(r.name, (r as any).actualHost || (r as any).hostName || undefined)} />
                                  <ActionBtn
                                    title="Console"
                                    icon={Terminal}
                                    onClick={() => setConsoleTarget({ ctr: r.name, host: (r as any).actualHost || (r as any).hostName || undefined })}
                                    disabled={!isRunning}
                                  />
                                  <ActionBtn
                                    title="Stats"
                                    icon={Activity}
                                    onClick={async () => {
                                      try {
                                        const targetHost = (r as any).actualHost || (r as any).hostName || host.name;
                                        const r2 = await fetch(
                                          `/api/containers/hosts/${encodeURIComponent(targetHost)}/${encodeURIComponent(r.name)}/stats`,
                                          { credentials: "include" }
                                        );
                                        if (r2.status === 401) {
                                          handle401();
                                          return;
                                        }
                                        const txt = await r2.text();
                                        setInfoModal({ title: `${r.name} (stats)`, text: txt || "(no data)" });
                                      } catch {
                                        setInfoModal({ title: `${r.name} (stats)`, text: "(failed to load stats)" });
                                      }
                                    }}
                                  />
                                  <ActionBtn title="Inspect" icon={Search} onClick={() => {
                                    // Navigate to the stack's actual host if different from current
                                    if (s.scopeName && s.scopeName !== host.name) {
                                      onHostChange(s.scopeName);
                                      setTimeout(() => onOpenStack(s.name), 100);
                                    } else {
                                      onOpenStack(s.name);
                                    }
                                  }} />
                                </>
                              )}
                            </div>
                          </td>
                          <td className="px-2 py-1.5 text-slate-300 hidden sm:table-cell">{r.created || "—"}</td>
                          <td className="px-2 py-1.5 text-slate-300 hidden sm:table-cell">{r.modified || "—"}</td>
                          <td className="px-2 py-1.5 text-slate-300 hidden md:table-cell">{r.ip || "—"}</td>
                          <td className="px-2 py-1.5 text-slate-300 hidden lg:table-cell pr-4 max-w-[100px]">
                            <div className="truncate" title={r.owner || "—"}>
                              {r.owner || "—"}
                            </div>
                          </td>
                        </tr>
                      );
                    })}
                  {(!s.rows || s.rows.filter((r) => matchRow(r, hostQuery)).length === 0) && (
                    <tr>
                      <td className="p-3 text-slate-500" colSpan={9}>
                        No containers or services.
                      </td>
                    </tr>
                  )}
                </tbody>
                </table>
              </div>
            </div>
          </CardContent>
        </Card>
      ))}

      <Card className="bg-slate-900/40 border-slate-800">
        <CardContent className="py-4 flex flex-wrap items-center gap-3 text-sm text-slate-300">
          Security by default:
          <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">AGE key never persisted</span>
          <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Decrypt to tmpfs only</span>
          <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Redacted logs</span>
          <span className="px-2 py-1 rounded bg-slate-800/60 border border-slate-700">Obscured paths</span>
        </CardContent>
      </Card>

      <NewStackDialog
        open={showNewStackDialog}
        onClose={() => setShowNewStackDialog(false)}
        onStackCreated={handleStackCreated}
        hosts={hosts}
        defaultHost={host.name}
        defaultScopeKind="host"
      />
    </div>
  );
}
