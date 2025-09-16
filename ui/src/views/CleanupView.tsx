// ui/src/views/CleanupView.tsx
import { useState, useEffect } from "react";
import { Card, CardContent } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Badge } from "@/components/ui/badge";
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
  RefreshCw,
  Loader2
} from "lucide-react";
import { Host } from "@/types";
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
  const [selectedHost, setSelectedHost] = useState<string>('');
  const [allHosts, setAllHosts] = useState(false);
  const [dryRun, setDryRun] = useState(true);
  const [activeJobs, setActiveJobs] = useState<Record<string, CleanupJob>>({});
  const [jobHistory, setJobHistory] = useState<CleanupJob[]>([]);
  const [spacePreviews, setSpacePreviews] = useState<Record<string, SpacePreview | AllHostsPreview>>({});
  const [previewLoading, setPreviewLoading] = useState<Record<string, boolean>>({});

  // Generate confirmation token for destructive operations
  const generateConfirmationToken = () => {
    return Math.random().toString(36).substring(2, 15);
  };

  // Fetch space preview for an operation
  const fetchSpacePreview = async (operation: CleanupOperation) => {
    const cacheKey = allHosts ? `${operation}-all-hosts` : `${operation}-${selectedHost}`;
    
    if (previewLoading[cacheKey]) return;
    
    setPreviewLoading(prev => ({ ...prev, [cacheKey]: true }));
    
    try {
      const endpoint = allHosts 
        ? `/api/cleanup/preview/${operation}/all-hosts`
        : `/api/cleanup/preview/${operation}/${selectedHost}`;
      
      const response = await fetch(endpoint, {
        credentials: 'include'
      });
      
      if (response.ok) {
        const preview = await response.json();
        setSpacePreviews(prev => ({ ...prev, [cacheKey]: preview }));
      }
    } catch (error) {
      console.error('Failed to fetch space preview:', error);
    } finally {
      setPreviewLoading(prev => ({ ...prev, [cacheKey]: false }));
    }
  };

  // Auto-fetch previews when target changes
  useEffect(() => {
    if ((allHosts || selectedHost) && hosts.length > 0) {
      // Fetch previews for all operations
      CLEANUP_OPERATIONS.forEach(op => {
        fetchSpacePreview(op.id);
      });
    }
  }, [allHosts, selectedHost, hosts.length]);

  // Clear previews when target changes
  useEffect(() => {
    setSpacePreviews({});
  }, [allHosts, selectedHost]);

  const executeCleanup = async (operation: CleanupOperation) => {
    if (!allHosts && !selectedHost) {
      alert('Please select a host');
      return;
    }

    const options: CleanupOptions = {
      dry_run: dryRun,
      force: operation === 'build-cache', // Build cache requires force
      exclude_filters: {},
      confirmation_token: dryRun ? '' : generateConfirmationToken()
    };

    try {
      const endpoint = allHosts 
        ? `/api/cleanup/all-hosts/${operation}`
        : `/api/cleanup/hosts/${selectedHost}/${operation}`;

      const response = await fetch(endpoint, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(options),
      });

      if (!response.ok) {
        throw new Error(`Failed to start cleanup: ${response.statusText}`);
      }

      const job: CleanupJob = await response.json();
      debugLog('Started cleanup job:', job);

      setActiveJobs(prev => ({
        ...prev,
        [job.id]: job
      }));

      // Start monitoring the job
      monitorJob(job.id);
      
    } catch (error) {
      console.error('Cleanup failed:', error);
      alert(`Cleanup failed: ${error}`);
    }
  };

  const monitorJob = async (jobId: string) => {
    const eventSource = new EventSource(`/api/cleanup/jobs/${jobId}/stream`);
    
    eventSource.onmessage = (event) => {
      if (event.type === 'progress') {
        const job: CleanupJob = JSON.parse(event.data);
        setActiveJobs(prev => ({
          ...prev,
          [jobId]: job
        }));
      }
    };

    eventSource.addEventListener('complete', (event) => {
      const job: CleanupJob = JSON.parse(event.data);
      setActiveJobs(prev => {
        const { [jobId]: completed, ...rest } = prev;
        return rest;
      });
      setJobHistory(prev => [job, ...prev.slice(0, 9)]); // Keep last 10 jobs
      eventSource.close();
    });

    eventSource.onerror = () => {
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
                  <Select value={selectedHost} onValueChange={setSelectedHost}>
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
          </div>
        </CardContent>
      </Card>

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
                    disabled={loading || (!allHosts && !selectedHost)}
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

      {/* Active Jobs */}
      {Object.keys(activeJobs).length > 0 && (
        <Card className="bg-slate-900/50 border-slate-800">
          <CardContent className="p-4">
            <h3 className="font-medium text-white mb-4 flex items-center gap-2">
              <RefreshCw className="h-4 w-4" />
              Active Cleanup Jobs
            </h3>
            <div className="space-y-3">
              {Object.values(activeJobs).map((job) => (
                <div key={job.id} className="p-3 bg-slate-800/50 rounded-lg">
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center gap-2">
                      {getStatusIcon(job.status)}
                      <span className="text-sm font-medium text-white">
                        {job.operation.replace('_', ' ')} - {job.target}
                      </span>
                      <Badge className={`text-xs ${getStatusColor(job.status)}`}>
                        {job.status}
                      </Badge>
                    </div>
                  </div>
                  
                  {job.status === 'running' && job.progress && (
                    <div className="space-y-2">
                      {job.progress.total_hosts && (
                        <div className="w-full bg-slate-800 rounded-full h-2">
                          <div 
                            className="bg-blue-500 h-2 rounded-full transition-all" 
                            style={{ width: `${(job.progress.completed_hosts || 0) / job.progress.total_hosts * 100}%` }}
                          ></div>
                        </div>
                      )}
                      <div className="text-xs text-slate-400">
                        {job.progress.current_operation} 
                        {job.progress.current_host && ` on ${job.progress.current_host}`}
                      </div>
                    </div>
                  )}
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Job History */}
      {jobHistory.length > 0 && (
        <Card className="bg-slate-900/50 border-slate-800">
          <CardContent className="p-4">
            <h3 className="font-medium text-white mb-4 flex items-center gap-2">
              <Clock className="h-4 w-4" />
              Recent Cleanup Results
            </h3>
            <div className="space-y-3">
              {jobHistory.map((job) => (
                <div key={job.id} className="p-3 bg-slate-800/50 rounded-lg">
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center gap-2">
                      {getStatusIcon(job.status)}
                      <span className="text-sm font-medium text-white">
                        {job.operation.replace('_', ' ')} - {job.target}
                      </span>
                      <Badge className={`text-xs ${getStatusColor(job.status)}`}>
                        {job.status}
                      </Badge>
                      {job.dry_run && (
                        <Badge variant="outline" className="text-xs bg-yellow-900/30 text-yellow-300 border-yellow-600">
                          Dry Run
                        </Badge>
                      )}
                    </div>
                    <span className="text-xs text-slate-500">
                      {new Date(job.created_at).toLocaleString()}
                    </span>
                  </div>
                  
                  {job.results && (
                    <div className="text-xs text-slate-400">
                      {Object.entries(job.results).map(([host, result]) => (
                        <div key={host} className="flex justify-between">
                          <span>{host}:</span>
                          <span className="text-green-400">{result.space_reclaimed}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}