// ui/src/views/CleanupView.tsx
import { useState, useEffect } from "react";
import { handle401 } from "@/utils/auth";
import { useParams, useNavigate } from "react-router-dom";
import { Card, CardContent } from "@/components/ui/card";
import { handle401 } from "@/utils/auth";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { handle401 } from "@/utils/auth";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
import { handle401 } from "@/utils/auth";
import { 
  Trash2, 
  HardDrive, 
  Package, 
  Container, 
  Network, 
  Database,
  AlertTriangle,
  CheckCircle,
  Clock,
  Loader2,
  ChevronDown,
  ChevronUp,
  Activity
} from "lucide-react";
import { Host } from "@/types";
import { handle401 } from "@/utils/auth";
import { debugLog } from "@/utils/logging";

// Types for cleanup operations
type CleanupOperation = 'system' | 'images' | 'containers' | 'volumes' | 'networks' | 'build-cache';

interface CleanupJob {
  id: string;
  operation: string;
  scope: 'single_host' | 'all_hosts';
  target: string;
  status: 'queued' | 'running' | 'completed' | 'failed';
  dry_run: boolean;
  created_at: string;
  progress?: {
    total_hosts?: number;
    completed_hosts?: number;
    current_host?: string;
    current_operation?: string;
  };
  results?: Record<string, {
    status: string;
    space_reclaimed: string;
    items_removed: Record<string, number>;
    errors: string[];
  }>;
}

interface SpacePreview {
  operation: string;
  estimated_size: string;
  estimated_bytes: number;
  item_count: Record<string, number>;
  details: string[];
  status: string;
  error?: string;
}

interface AllHostsPreview {
  operation: string;
  total_bytes: number;
  total_size: string;
  host_previews: Record<string, SpacePreview>;
  total_item_count: Record<string, number>;
}

interface CleanupOptions {
  dry_run: boolean;
  force: boolean;
  exclude_filters: Record<string, string[]>;
  confirmation_token: string;
}

const CLEANUP_OPERATIONS = [
  {
    id: 'build-cache' as CleanupOperation,
    name: 'Build Cache',
    description: 'Remove Docker build cache (most space)',
    icon: HardDrive,
    primary: true
  },
  {
    id: 'system' as CleanupOperation,
    name: 'System Prune',
    description: 'Remove all unused containers, networks, images',
    icon: Trash2,
    primary: false
  },
  {
    id: 'images' as CleanupOperation,
    name: 'Images',
    description: 'Remove unused images',
    icon: Package,
    primary: false
  },
  {
    id: 'containers' as CleanupOperation,
    name: 'Containers',
    description: 'Remove stopped containers',
    icon: Container,
    primary: false
  },
  {
    id: 'volumes' as CleanupOperation,
    name: 'Volumes',
    description: 'Remove unused volumes',
    icon: Database,
    primary: false
  },
  {
    id: 'networks' as CleanupOperation,
    name: 'Networks',
    description: 'Remove unused networks',
    icon: Network,
    primary: false
  }
];

export default function CleanupView({
  hosts,
  loading
}: {
  hosts: Host[];
  loading: boolean;
}) {
  const { hostName: urlHostName } = useParams<{ hostName: string }>();
  const navigate = useNavigate();
  
  // Get host from URL parameter, fallback to localStorage, then first host
  const getInitialHost = () => {
    if (urlHostName) return decodeURIComponent(urlHostName);
    const saved = localStorage.getItem("selectedHost");
    if (saved && hosts.some(h => h.name === saved)) return saved;
    return hosts.length > 0 ? hosts[0].name : '';
  };
  
  const [selectedHost, setSelectedHost] = useState<string>('');
  const [allHosts, setAllHosts] = useState(false);
  const [dryRun, setDryRun] = useState(true);
  const [isExecuting, setIsExecuting] = useState(false);
  const [jobHistory, setJobHistory] = useState<CleanupJob[]>([]);
  const [spacePreviews, setSpacePreviews] = useState<Record<string, SpacePreview | AllHostsPreview>>({});
  const [previewLoading, setPreviewLoading] = useState<Record<string, boolean>>({});
  const [cleanupEvents, setCleanupEvents] = useState<Array<{id: string, time: Date, message: string, type: 'info' | 'success' | 'warning' | 'error' | 'progress'}>>([]);
  const [showEvents, setShowEvents] = useState(false);
  
  // Initialize selected host from URL or saved preference
  useEffect(() => {
    if (hosts.length > 0) {
      const initial = getInitialHost();
      if (initial && initial !== selectedHost) {
        setSelectedHost(initial);
      }
    }
  }, [hosts, urlHostName]);
  
  // Handle host selection change
  const handleHostChange = (newHost: string) => {
    setSelectedHost(newHost);
    localStorage.setItem("selectedHost", newHost);
    // Navigate to the new URL
    navigate(`/hosts/${encodeURIComponent(newHost)}/cleanup`);
  };

  // Generate confirmation token for destructive operations
  const generateConfirmationToken = () => {
    return Math.random().toString(36).substring(2, 15);
  };

  // Add event to the events list
  const addEvent = (message: string, type: 'info' | 'success' | 'warning' | 'error' | 'progress' = 'info') => {
    const event = {
      id: Math.random().toString(36).substr(2, 9),
      time: new Date(),
      message,
      type
    };
    setCleanupEvents(prev => [...prev, event].slice(-50)); // Keep last 50 events, append new ones
    setShowEvents(true); // Auto-show events when new ones come in
  };

  // Fetch space preview for an operation
  const fetchSpacePreview = async (operation: CleanupOperation) => {
    const cacheKey = allHosts ? `${operation}-all-hosts` : `${operation}-${selectedHost}`;
    
    if (previewLoading[cacheKey]) return;
    
    setPreviewLoading(prev => ({ ...prev, [cacheKey]: true }));
    addEvent(`Starting preview analysis for ${operation}...`, 'info');
    
    try {
      const endpoint = allHosts  ? `/api/cleanup/global/preview/${operation}` : `/api/cleanup/hosts/${encodeURIComponent(selectedHost)}/preview/${operation}`;
      
      const response = await fetch(endpoint, {
        credentials: 'include'
      });
      
      if (response.status === 401) {
        handle401();
        return;
      }
      
      if (response.ok) {
        const preview = await response.json();
        setSpacePreviews(prev => ({ ...prev, [cacheKey]: preview }));
        
        // Add detailed event about what was found
        const itemSummary = Object.entries(preview.item_count || {}) .filter(([_, count]) => count > 0) .map(([key, count]) => `${count} ${key}`) .join(', ');
        
        if (itemSummary) {
          addEvent(`Preview complete: Found ${itemSummary} (${preview.estimated_size || '0B'} total)`, 'success');
        } else {
          addEvent(`Preview complete: Nothing to clean for ${operation}`, 'success');
        }
        
        // Add details if available
        if (preview.details && preview.details.length > 0) {
          preview.details.slice(0, 5).forEach(detail => {
            addEvent(`  • ${detail}`, 'progress');
          });
          if (preview.details.length > 5) {
            addEvent(`  ... and ${preview.details.length - 5} more items`, 'progress');
          }
        }
      } else {
        addEvent(`Preview failed for ${operation}: ${response.statusText}`, 'error');
      }
    } catch (error) {
      console.error('Failed to fetch space preview:', error);
      addEvent(`Preview error for ${operation}: ${error}`, 'error');
    } finally {
      setPreviewLoading(prev => ({ ...prev, [cacheKey]: false }));
    }
  };

  // Fetch all previews when host or mode changes
  useEffect(() => {
    if (!selectedHost && !allHosts) return;
    
    // Clear existing previews first
    setSpacePreviews({});
    
    // Fetch preview for each operation
    const operations: CleanupOperation[] = ['system', 'images', 'containers', 'volumes', 'networks', 'build-cache'];
    operations.forEach(op => {
      fetchSpacePreview(op);
    });
  }, [allHosts, selectedHost]);

  const executeCleanup = async (operation: CleanupOperation) => {
    if (!allHosts && !selectedHost) {
      addEvent('Please select a host before cleanup', 'warning');
      return;
    }

    // If dryRun is true, call the GET preview endpoint instead
    if (dryRun) {
      await fetchSpacePreview(operation);
      return;
    }

    // Only execute actual cleanup when dryRun is false
    const options: CleanupOptions = {
      dry_run: false,
      force: operation === 'build-cache', // Build cache requires force
      exclude_filters: {},
      confirmation_token: generateConfirmationToken()
    };

    setIsExecuting(true);
    try {
      const endpoint = allHosts  ? `/api/cleanup/global/${operation}` : `/api/cleanup/hosts/${encodeURIComponent(selectedHost)}/${operation}`;

      const response = await fetch(endpoint, {
        method: 'POST',
        credentials: 'include',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(options),
      });

      if (response.status === 401) {
        handle401();
        return;
      }

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(errorText || response.statusText);
      }

      const job: CleanupJob = await response.json();
      debugLog('Started cleanup job:', job);

      // Start monitoring the job
      addEvent(`✅ Cleanup job ${job.id.slice(0, 8)} started`, 'success');
      addEvent(`🚀 Executing ${operation} cleanup...`, 'info');
      monitorJob(job.id);
      
    } catch (error) {
      console.error('Cleanup failed:', error);
      addEvent(`Cleanup failed: ${error}`, 'error');
    } finally {
      setIsExecuting(false);
    }
  };

  const monitorJob = async (jobId: string) => {
    addEvent(`Starting cleanup job monitor for ${jobId.slice(0, 8)}...`, 'info');
    
    // Small delay to ensure backend job is ready
    await new Promise(resolve => setTimeout(resolve, 100));
    
    const eventSource = new EventSource(`/api/cleanup/jobs/${jobId}/stream`);
    let hasReceivedData = false;
    
    eventSource.onmessage = (event) => {
      hasReceivedData = true;
      debugLog('SSE onmessage received:', { type: event.type, data: event.data });
      
      // The default message event handler
      try {
        const data = JSON.parse(event.data);
        debugLog('Parsed SSE data:', data);
        
        // Handle job progress updates
        if (data.progress && typeof data.progress === 'object') {
          const progress = data.progress as any;
          if (progress.message && !progress.message.includes('heartbeat')) {
            // Only show the message if it's new or different
            addEvent(progress.message, 
              progress.phase === 'completed' ? 'success' : 
              progress.phase === 'failed' ? 'error' : 'progress'
            );
          }
          if (progress.current && progress.total) {
            addEvent(`Progress: ${progress.current}/${progress.total} items processed`, 'progress');
          }
        } else if (data.message) {
          // Fallback to simple message
          addEvent(data.message, 'progress');
        }
      } catch (e) {
        // Ignore parse errors for heartbeats
        if (!event.data.includes('heartbeat')) {
          debugLog('Failed to parse SSE message:', e);
        }
      }
    };

    eventSource.addEventListener('complete', (event) => {
      debugLog('SSE complete event received:', event.data);
      const job: CleanupJob = JSON.parse(event.data);
      setJobHistory(prev => [job, ...prev.slice(0, 9)]); // Keep last 10 jobs
      
      // Add completion event with results
      if (job.results && typeof job.results === 'object') {
        const results = job.results as any;
        if (results.space_reclaimed) {
          addEvent(`✅ Cleanup completed! Reclaimed ${results.space_reclaimed}`, 'success');
        } else {
          addEvent(`✅ Cleanup job ${jobId.slice(0, 8)} completed`, 'success');
        }
        
        // Add summary of what was removed
        if (results.removed) {
          Object.entries(results.removed).forEach(([key, value]) => {
            if (value && value !== 0) {
              addEvent(`  • Removed ${value} ${key}`, 'progress');
            }
          });
        }
      } else {
        addEvent(`✅ Cleanup job ${jobId.slice(0, 8)} completed`, 'success');
      }
      
      // Refresh the preview for the operation that was just cleaned
      if (job.operation) {
        // Map operation names to cleanup operation types
        let operationType: CleanupOperation | null = null;
        switch (job.operation) {
          case 'system_prune':
            operationType = 'system';
            break;
          case 'image_prune':
            operationType = 'images';
            break;
          case 'container_prune':
            operationType = 'containers';
            break;
          case 'volume_prune':
            operationType = 'volumes';
            break;
          case 'network_prune':
            operationType = 'networks';
            break;
          case 'build_cache_prune':
            operationType = 'build-cache';
            break;
        }
        
        if (operationType) {
          addEvent('Refreshing preview data...', 'info');
          fetchSpacePreview(operationType);
        }
      }
      
      eventSource.close();
    });
    
    // Add handler for connection established
    eventSource.addEventListener('connected', (event) => {
      debugLog('SSE connected event received:', event.data);
      hasReceivedData = true;
    });
    
    // Add handlers for other SSE event types
    eventSource.addEventListener('progress', (event) => {
      debugLog('SSE progress event received:', event.data);
      hasReceivedData = true;
      try {
        const data = JSON.parse(event.data);
        
        // Handle progress field if present
        if (data.progress && typeof data.progress === 'object') {
          const progress = data.progress as any;
          if (progress.message) {
            addEvent(progress.message, 
              progress.phase === 'completed' ? 'success' : 
              progress.phase === 'failed' ? 'error' : 'progress'
            );
          }
        } else if (data.message) {
          // Fallback to simple message
          addEvent(data.message, 'progress');
        }
      } catch (e) {
        debugLog('Failed to parse progress event:', e);
      }
    });
    
    eventSource.addEventListener('info', (event) => {
      debugLog('SSE info event received:', event.data);
      try {
        const data = JSON.parse(event.data);
        if (data.message) {
          addEvent(data.message, 'info');
        }
      } catch (e) {
        debugLog('Failed to parse info event:', e);
      }
    });

    eventSource.onerror = (error) => {
      debugLog('SSE error:', error, 'readyState:', eventSource.readyState, 'hasReceivedData:', hasReceivedData);
      // Only show warning if we had an actual connection issue (not a normal close)
      // EventSource.CONNECTING = 0, EventSource.OPEN = 1, EventSource.CLOSED = 2
      if (eventSource.readyState === EventSource.CONNECTING && !hasReceivedData) {
        // Failed to establish initial connection
        addEvent(`⚠️ Failed to connect to cleanup job ${jobId.slice(0, 8)}`, 'warning');
      }
      // Don't show warnings for normal closures or when we've received data
      // The onerror event fires even for normal closes, so we need to be careful
      eventSource.close();
    };
  };

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'queued': return <Clock className="h-4 w-4 text-blue-400" />;
      case 'running': return <Loader2 className="h-4 w-4 text-blue-400 animate-spin" />;
      case 'completed': return <CheckCircle className="h-4 w-4 text-green-400" />;
      case 'failed': return <AlertTriangle className="h-4 w-4 text-red-400" />;
      default: return <Clock className="h-4 w-4 text-gray-400" />;
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'queued': return 'bg-blue-900/50 text-blue-300 border-blue-700';
      case 'running': return 'bg-blue-900/50 text-blue-300 border-blue-700';
      case 'completed': return 'bg-green-900/50 text-green-300 border-green-700';
      case 'failed': return 'bg-red-900/50 text-red-300 border-red-700';
      default: return 'bg-gray-900/50 text-gray-300 border-gray-700';
    }
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-4">
        <div className="text-lg font-semibold text-white">Docker Cleanup</div>
        <div className="px-3 py-2 bg-slate-900/60 border border-slate-800 rounded-lg flex items-center gap-2">
          <Trash2 className="h-4 w-4 text-slate-400" />
          <span className="text-sm text-slate-300">Cache & Temp Data</span>
        </div>
      </div>

      {/* Target Selection */}
      <Card className="bg-slate-900/50 border-slate-800">
        <CardContent className="p-4">
          <div className="space-y-4">
            <div className="flex items-center gap-4">
              <label className="text-sm font-medium text-slate-300">Target:</label>
              <div className="flex items-center gap-3">
                <div className="flex items-center gap-2">
                  <Switch
                    checked={allHosts}
                    onCheckedChange={setAllHosts}
                    className="data-[state=checked]:bg-blue-600"
                  />
                  <span className="text-sm text-slate-300">All Hosts</span>
                </div>
                {!allHosts && (
                  <Select value={selectedHost} onValueChange={handleHostChange}>
                    <SelectTrigger className="w-48 bg-slate-900 border-slate-700">
                      <SelectValue placeholder="Select host..." />
                    </SelectTrigger>
                    <SelectContent className="bg-slate-900 border-slate-700">
                      {hosts.map((host) => (
                        <SelectItem key={host.name} value={host.name}>
                          {host.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              </div>
            </div>

          </div>

          <div className="flex items-center gap-4">
            <label className="text-sm font-medium text-slate-300">Mode:</label>
            <div className="flex items-center gap-2">
              <Switch
                checked={dryRun}
                onCheckedChange={setDryRun}
                className="data-[state=checked]:bg-blue-600"
              />
              <span className="text-sm text-slate-300">
                {dryRun ? 'Dry Run (Preview)' : 'Execute Cleanup'}
              </span>
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Events Card - Collapsible like deployment events */}
      {cleanupEvents.length > 0 && (
        <Card className="bg-slate-900/50 border-slate-800">
          <CardContent className="p-4">
            <div 
              className="flex items-center justify-between cursor-pointer"
              onClick={() => setShowEvents(!showEvents)}
            >
              <div className="flex items-center gap-2">
                <Activity className="h-4 w-4 text-blue-400" />
                <span className="font-medium text-white">Cleanup Events</span>
                <Badge variant="outline" className="text-xs">
                  {cleanupEvents.length}
                </Badge>
              </div>
              {showEvents ? (
                <ChevronUp className="h-4 w-4 text-slate-400" />
              ) : (
                <ChevronDown className="h-4 w-4 text-slate-400" />
              )}
            </div>
            
            {showEvents && (
              <div className="mt-3 space-y-1 max-h-64 overflow-y-auto">
                {cleanupEvents.map(event => (
                  <div 
                    key={event.id}
                    className={`text-xs font-mono p-2 rounded ${
                      event.type === 'error' ? 'bg-red-900/20 text-red-300' :
                      event.type === 'warning' ? 'bg-yellow-900/20 text-yellow-300' :
                      event.type === 'success' ? 'bg-green-900/20 text-green-300' :
                      event.type === 'progress' ? 'bg-blue-900/20 text-blue-300' :
                      'bg-slate-800/50 text-slate-400'
                    }`}
                  >
                    <span className="text-slate-500">
                      [{event.time.toLocaleTimeString()}]
                    </span>{' '}
                    {event.message}
                  </div>
                ))}
              </div>
            )}
            
            {showEvents && cleanupEvents.length > 0 && (
              <Button
                variant="ghost"
                size="sm"
                className="mt-2 text-xs text-slate-400 hover:text-slate-200"
                onClick={(e) => {
                  e.stopPropagation();
                  setCleanupEvents([]);
                }}
              >
                Clear Events
              </Button>
            )}
          </CardContent>
        </Card>
      )}

      {/* Cleanup Operations */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        {CLEANUP_OPERATIONS.map((op) => {
          const Icon = op.icon;
          const cacheKey = allHosts ? `${op.id}-all-hosts` : `${op.id}-${selectedHost}`;
          const preview = spacePreviews[cacheKey] as SpacePreview | AllHostsPreview;
          const isPreviewLoading = previewLoading[cacheKey];

          return (
            <Card key={op.id} className={`bg-slate-900/50 border-slate-800 ${op.primary ? 'ring-2 ring-blue-500/30' : ''}`}>
              <CardContent className="p-4">
                <div className="space-y-3">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <Icon className={`h-5 w-5 ${op.primary ? 'text-blue-400' : 'text-slate-400'}`} />
                      <div>
                        <h3 className="font-medium text-white">{op.name}</h3>
                        {op.primary && (
                          <Badge variant="outline" className="text-xs bg-blue-900/30 text-blue-300 border-blue-600">
                            Recommended
                          </Badge>
                        )}
                      </div>
                    </div>
                    {/* Space preview display */}
                    <div className="text-right text-sm">
                      {isPreviewLoading ? (
                        <div className="flex items-center gap-1 text-slate-400">
                          <Loader2 className="h-3 w-3 animate-spin" />
                          <span className="text-xs">Analyzing...</span>
                        </div>
                      ) : preview ? (
                        <div>
                          <div className="text-green-400 font-medium">
                            {'total_size' in preview ? preview.total_size : preview.estimated_size}
                          </div>
                          {Object.entries(('total_item_count' in preview ? preview.total_item_count : preview.item_count) || {}).map(([key, count]) => (
                            <div key={key} className="text-xs text-slate-500">
                              {count} {key.replace('_', ' ')}
                            </div>
                          ))}
                        </div>
                      ) : (
                        (allHosts || selectedHost) && (
                          <div className="text-xs text-slate-500">
                            No data
                          </div>
                        )
                      )}
                    </div>
                  </div>
                  <p className="text-sm text-slate-400">{op.description}</p>
                  {/* Show preview details if available */}
                  {preview && preview.status === 'success' && (
                    <div className="text-xs text-slate-500">
                      {'host_previews' in preview ? (
                        <div>
                          {Object.keys(preview.host_previews).length} hosts • {Object.values(preview.host_previews).filter(p => p.status === 'success').length} accessible
                        </div>
                      ) : (
                        preview.details && preview.details.length > 0 && (
                          <div className="max-h-20 overflow-y-auto">
                            {preview.details.map((detail, i) => (
                              <div key={i}>{detail}</div>
                            ))}
                          </div>
                        )
                      )}
                    </div>
                  )}
                  <Button
                    onClick={() => executeCleanup(op.id)}
                    disabled={isExecuting || (!allHosts && !selectedHost)}
                    className={`w-full ${op.primary ? 'bg-blue-600 hover:bg-blue-700' : 'bg-slate-700 hover:bg-slate-600'}`}
                    size="sm"
                  >
                    {dryRun ? 'Preview' : 'Clean'} {op.name}
                  </Button>
                </div>
              </CardContent>
            </Card>
          );
        })}
      </div>


    </div>
  );
}