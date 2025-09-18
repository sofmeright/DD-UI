import { useState, useEffect, useRef, useCallback } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { SearchAddon } from "@xterm/addon-search";
import "@xterm/xterm/css/xterm.css";
import {
  Search,
  X,
  Pause,
  Play,
  Download,
  Filter,
  AlertCircle,
  Info,
  AlertTriangle,
  XCircle,
  ChevronDown,
  ChevronRight,
  Monitor,
  Server,
  Layers,
  Box,
  RefreshCw,
  Trash2,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { debugLog, infoLog, warnLog, errorLog } from "@/utils/logging";

interface LogEntry {
  id?: number;
  timestamp: string;
  hostname: string;
  stack_name?: string;
  service_name: string;
  container_id: string;
  container_name?: string;
  level: string;
  source: string;
  message: string;
  labels?: Record<string, string>;
}

interface LogFilter {
  hostnames?: string[];
  stacks?: string[];
  services?: string[];
  containers?: string[];
  levels?: string[];
  since?: string;
  until?: string;
  search?: string;
  limit?: number;
  follow?: boolean;
}

interface LogStats {
  total: number;
  byLevel: Record<string, number>;
  byHost: Record<string, number>;
  byStack: Record<string, number>;
}

interface LogSource {
  hosts: string[];
  stacks: string[];
  containers: Array<{ name: string; host: string; stack?: string }>;
}

export default function LoggingView() {
  const [selectedHost, setSelectedHost] = useState<string>("all");
  const [selectedStack, setSelectedStack] = useState<string>("all");
  const [selectedContainer, setSelectedContainer] = useState<string>("all");
  const [logLevels, setLogLevels] = useState<string[]>(["INFO", "WARN", "ERROR", "DEBUG"]);
  const [searchQuery, setSearchQuery] = useState("");
  const [isFollowing, setIsFollowing] = useState(true);
  const [isConnected, setIsConnected] = useState(false);
  const [logBuffer, setLogBuffer] = useState<LogEntry[]>([]);
  const [filteredLogs, setFilteredLogs] = useState<LogEntry[]>([]); // Track filtered logs separately
  const [stats, setStats] = useState<LogStats>({
    total: 0,
    byLevel: {},
    byHost: {},
    byStack: {},
  });
  
  // Dropdown open states
  const [hostDropdownOpen, setHostDropdownOpen] = useState(false);
  const [stackDropdownOpen, setStackDropdownOpen] = useState(false);
  const [containerDropdownOpen, setContainerDropdownOpen] = useState(false);

  // Raw data from backend
  const [logSources, setLogSources] = useState<LogSource>({
    hosts: [],
    stacks: [],
    containers: [],
  });

  // Cascading filtered options
  const [filteredStacks, setFilteredStacks] = useState<string[]>([]);
  const [filteredContainers, setFilteredContainers] = useState<Array<{ name: string; host: string; stack?: string }>>([]);

  // Terminal setup
  const terminalRef = useRef<HTMLDivElement>(null);
  const terminalInstance = useRef<Terminal | null>(null);
  const fitAddon = useRef<FitAddon | null>(null);
  const searchAddon = useRef<SearchAddon | null>(null);
  const eventSourceRef = useRef<EventSource | null>(null);

  // Initialize terminal
  useEffect(() => {
    if (!terminalRef.current) return;

    const terminal = new Terminal({
      theme: {
        background: "#0f172a", // slate-900
        foreground: "#cbd5e1", // slate-300
        cursor: "#94a3b8", // slate-400
        black: "#020617", // slate-950
        red: "#ef4444",
        green: "#10b981",
        yellow: "#f59e0b",
        blue: "#3b82f6",
        magenta: "#8b5cf6",
        cyan: "#06b6d4",
        white: "#e2e8f0", // slate-200
        brightBlack: "#475569", // slate-600
        brightRed: "#f87171",
        brightGreen: "#34d399",
        brightYellow: "#fbbf24",
        brightBlue: "#60a5fa",
        brightMagenta: "#a78bfa",
        brightCyan: "#22d3ee",
        brightWhite: "#f8fafc", // slate-50
      },
      fontFamily: "JetBrains Mono, Monaco, Consolas, monospace",
      fontSize: 13,
      lineHeight: 1.5,
      convertEol: true,
      cursorBlink: false,
      disableStdin: true,
      scrollback: 10000,
    });

    const fit = new FitAddon();
    const search = new SearchAddon();

    fitAddon.current = fit;
    searchAddon.current = search;

    terminal.loadAddon(fit);
    terminal.loadAddon(search);
    terminal.open(terminalRef.current);

    fit.fit();
    terminalInstance.current = terminal;

    // Write welcome message
    terminal.writeln("\x1b[36m📊 DD-UI Logging System\x1b[0m");
    terminal.writeln("\x1b[90m" + "=".repeat(80) + "\x1b[0m");
    terminal.writeln("");
    terminal.writeln("\x1b[33m⚡ Connecting to log stream...\x1b[0m");

    // Handle window resize
    const handleResize = () => {
      if (fitAddon.current) {
        fitAddon.current.fit();
      }
    };
    window.addEventListener("resize", handleResize);

    return () => {
      window.removeEventListener("resize", handleResize);
      terminal.dispose();
    };
  }, []);

  // Fetch available log sources
  useEffect(() => {
    fetch("/api/logs/sources")
      .then((res) => res.json())
      .then((data: LogSource) => {
        debugLog("Fetched log sources:", {
          hosts: data.hosts?.length || 0,
          stacks: data.stacks?.length || 0,
          containers: data.containers?.length || 0
        });
        setLogSources(data);
        setFilteredStacks(data.stacks || []);
        setFilteredContainers(data.containers || []);
      })
      .catch((err) => {
        errorLog("Failed to fetch log sources:", err);
      });
  }, []);

  // Cascade filters: when host changes, filter stacks and containers
  useEffect(() => {
    debugLog("Host selection changed:", selectedHost);
    
    if (selectedHost === "all") {
      setFilteredStacks(logSources.stacks);
      setFilteredContainers(logSources.containers);
    } else {
      // Filter stacks based on containers in the selected host
      const stacksInHost = [...new Set(
        logSources.containers
          .filter(c => c.host === selectedHost && c.stack)
          .map(c => c.stack!)
      )];
      debugLog("Stacks available for host", selectedHost, ":", stacksInHost);
      setFilteredStacks(stacksInHost);
      
      // Filter containers by host
      setFilteredContainers(
        logSources.containers.filter(c => c.host === selectedHost)
      );
    }
    
    // Don't reset stack here - we'll check it after the filtered stacks are set
  }, [selectedHost, logSources]);

  // Check if selected stack is still valid after filtering
  useEffect(() => {
    if (selectedStack !== "all" && !filteredStacks.includes(selectedStack)) {
      setSelectedStack("all");
    }
  }, [filteredStacks, selectedStack]);

  // When stack changes, filter containers
  useEffect(() => {
    debugLog("Stack selection changed:", selectedStack, "Host:", selectedHost);
    
    let filtered: Array<{ name: string; host: string; stack?: string }> = [];
    
    if (selectedHost === "all" && selectedStack === "all") {
      filtered = logSources.containers;
    } else if (selectedHost !== "all" && selectedStack === "all") {
      filtered = logSources.containers.filter(c => c.host === selectedHost);
    } else if (selectedHost === "all" && selectedStack !== "all") {
      filtered = logSources.containers.filter(c => c.stack === selectedStack);
    } else {
      filtered = logSources.containers.filter(
        c => c.host === selectedHost && c.stack === selectedStack
      );
    }
    
    debugLog("Filtered containers for stack", selectedStack, ":", filtered.map(c => c.name));
    setFilteredContainers(filtered);
    
    // Reset container selection if it's no longer valid
    if (selectedContainer !== "all" && !filtered.some(c => c.name === selectedContainer)) {
      debugLog("Resetting container selection, current:", selectedContainer);
      setSelectedContainer("all");
    }
  }, [selectedStack, selectedHost, logSources]);

  // Connect to log stream
  const connectToLogStream = useCallback(() => {
    debugLog("connectToLogStream called with log levels:", logLevels);
    
    // Disconnect existing stream
    if (eventSourceRef.current) {
      debugLog("Closing existing log stream connection");
      eventSourceRef.current.close();
    }

    // Clear filtered logs and terminal when reconnecting with new filters
    setFilteredLogs([]);
    if (terminalInstance.current) {
      terminalInstance.current.clear();
      terminalInstance.current.writeln("\x1b[36m📊 DD-UI Logging System\x1b[0m");
      terminalInstance.current.writeln("\x1b[90m" + "=".repeat(80) + "\x1b[0m");
      terminalInstance.current.writeln("");
      terminalInstance.current.writeln("\x1b[33m⚡ Reconnecting with new filters...\x1b[0m");
    }

    // Build filter parameters
    const filters: LogFilter = {
      levels: logLevels,
      follow: isFollowing,
    };

    if (selectedHost !== "all") {
      filters.hostnames = [selectedHost];
    }
    if (selectedStack !== "all") {
      filters.stacks = [selectedStack];
    }
    if (selectedContainer !== "all") {
      filters.containers = [selectedContainer];
    }
    if (searchQuery) {
      filters.search = searchQuery;
    }

    const params = new URLSearchParams();
    Object.entries(filters).forEach(([key, value]) => {
      if (value !== undefined) {
        if (Array.isArray(value)) {
          params.append(key, value.join(","));
        } else {
          params.append(key, String(value));
        }
      }
    });

    debugLog("Connecting to log stream with filters:", filters, "URL params:", params.toString());

    const eventSource = new EventSource(
      `/api/logs/stream?${params.toString()}`
    );

    eventSource.onopen = () => {
      setIsConnected(true);
      infoLog("Log stream connected");
      if (terminalInstance.current) {
        terminalInstance.current.writeln(
          "\x1b[32m✓ Connected to log stream\x1b[0m"
        );
      }
    };

    eventSource.onmessage = (event) => {
      try {
        const logEntry: LogEntry = JSON.parse(event.data);
        processLogEntry(logEntry);
      } catch (error) {
        errorLog("Failed to parse log entry:", error);
      }
    };

    eventSource.onerror = (error) => {
      setIsConnected(false);
      
      // Only log as error if it's not a normal close
      if (eventSource.readyState === EventSource.CLOSED) {
        debugLog("Log stream connection closed, will reconnect");
      } else {
        warnLog("Log stream connection interrupted, will reconnect");
      }
      
      if (terminalInstance.current) {
        terminalInstance.current.writeln(
          "\x1b[33m⟳ Connection interrupted. Reconnecting in 3 seconds...\x1b[0m"
        );
      }
      
      // Close the current connection
      eventSource.close();
      
      setTimeout(() => {
        if (isFollowing) {
          connectToLogStream();
        }
      }, 3000);
    };

    eventSourceRef.current = eventSource;
  }, [selectedHost, selectedStack, selectedContainer, logLevels, searchQuery, isFollowing]);

  // Check if a log entry matches current filters (for client-side filtering)
  const matchesCurrentFilters = useCallback((entry: LogEntry) => {
    // Check log levels
    if (!logLevels.includes(entry.level)) {
      return false;
    }
    
    // Check host
    if (selectedHost !== "all" && entry.hostname !== selectedHost) {
      return false;
    }
    
    // Check stack
    if (selectedStack !== "all" && entry.stack_name !== selectedStack) {
      return false;
    }
    
    // Check container
    if (selectedContainer !== "all" && entry.container_name !== selectedContainer) {
      return false;
    }
    
    // Check search query
    if (searchQuery && !entry.message.toLowerCase().includes(searchQuery.toLowerCase())) {
      return false;
    }
    
    return true;
  }, [selectedHost, selectedStack, selectedContainer, logLevels, searchQuery]);

  // Process and display log entry
  const processLogEntry = useCallback(
    (entry: LogEntry) => {
      // Update full buffer
      setLogBuffer((prev) => {
        const updated = [...prev, entry].slice(-1000); // Keep last 1000 entries
        return updated;
      });

      // Update filtered logs if it matches current filters
      if (matchesCurrentFilters(entry)) {
        setFilteredLogs((prev) => {
          const updated = [...prev, entry].slice(-1000); // Keep last 1000 filtered entries
          return updated;
        });

        // Update stats only for visible logs
        setStats((prev) => ({
          total: prev.total + 1,
          byLevel: {
            ...prev.byLevel,
            [entry.level]: (prev.byLevel[entry.level] || 0) + 1,
          },
          byHost: {
            ...prev.byHost,
            [entry.hostname]: (prev.byHost[entry.hostname] || 0) + 1,
          },
          byStack: entry.stack_name
            ? {
                ...prev.byStack,
                [entry.stack_name]: (prev.byStack[entry.stack_name] || 0) + 1,
              }
            : prev.byStack,
        }));

        // Write to terminal only if visible
        if (terminalInstance.current) {
          const timestamp = new Date(entry.timestamp).toLocaleTimeString();
          const levelColor = {
            ERROR: "\x1b[31m", // Red
            WARN: "\x1b[33m",  // Yellow
            INFO: "\x1b[32m",  // Green
            DEBUG: "\x1b[36m", // Cyan
          }[entry.level] || "\x1b[37m"; // Default white

          const levelIcon = {
            ERROR: "✗",
            WARN: "⚠",
            INFO: "ℹ",
            DEBUG: "⚡",
          }[entry.level] || "•";

          const line = `${timestamp} ${levelColor}${levelIcon} [${entry.level}]\x1b[0m \x1b[90m${entry.hostname}/${entry.container_name || entry.service_name}:\x1b[0m ${entry.message}`;
          terminalInstance.current.writeln(line);
        }
      }
    },
    [matchesCurrentFilters]
  );

  // Start/stop following logs and reconnect when filters change
  useEffect(() => {
    if (isFollowing) {
      connectToLogStream();
    } else {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
        eventSourceRef.current = null;
      }
    }

    return () => {
      if (eventSourceRef.current) {
        eventSourceRef.current.close();
      }
    };
  }, [isFollowing, connectToLogStream]);

  // Reconnect when log levels change
  useEffect(() => {
    debugLog("Log levels changed:", logLevels);
    if (isFollowing && eventSourceRef.current) {
      debugLog("Reconnecting due to log level change");
      connectToLogStream();
    }
  }, [logLevels, isFollowing, connectToLogStream]);

  // Clear terminal and filtered logs only
  const clearTerminal = () => {
    if (terminalInstance.current) {
      terminalInstance.current.clear();
      // Clear only the filtered logs, not the full buffer
      setFilteredLogs([]);
      // Reset stats for visible scope
      setStats({
        total: 0,
        byLevel: {},
        byHost: {},
        byStack: {},
      });
      
      // Write a message indicating scope
      terminalInstance.current.writeln("\x1b[33m⟳ Cleared visible logs\x1b[0m");
      terminalInstance.current.writeln(`\x1b[90mScope: ${selectedHost === "all" ? "All Hosts" : selectedHost}${selectedStack !== "all" ? ` / ${selectedStack}` : ""}${selectedContainer !== "all" ? ` / ${selectedContainer}` : ""}\x1b[0m`);
      terminalInstance.current.writeln("");
    }
  };

  // Export only filtered/visible logs
  const exportLogs = () => {
    if (filteredLogs.length === 0) {
      // No logs to export
      if (terminalInstance.current) {
        terminalInstance.current.writeln("\x1b[33m⚠ No visible logs to export\x1b[0m");
      }
      return;
    }
    
    // Build filename with scope information
    const scopeParts = [];
    if (selectedHost !== "all") scopeParts.push(selectedHost);
    if (selectedStack !== "all") scopeParts.push(selectedStack);
    if (selectedContainer !== "all") scopeParts.push(selectedContainer);
    const scopeStr = scopeParts.length > 0 ? `-${scopeParts.join("-")}` : "";
    
    const content = filteredLogs
      .map((entry) => {
        return `${entry.timestamp} [${entry.level}] ${entry.hostname}/${entry.container_name || entry.service_name}: ${entry.message}`;
      })
      .join("\n");

    const blob = new Blob([content], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `logs${scopeStr}-${new Date().toISOString()}.txt`;
    a.click();
    URL.revokeObjectURL(url);
    
    // Show confirmation
    if (terminalInstance.current) {
      terminalInstance.current.writeln(`\x1b[32m✓ Exported ${filteredLogs.length} visible log entries\x1b[0m`);
    }
  };

  // Search in terminal
  const [searchOpen, setSearchOpen] = useState(false);
  const [searchTerm, setSearchTerm] = useState("");

  const handleSearch = (term: string) => {
    if (searchAddon.current && term) {
      searchAddon.current.findNext(term);
    }
  };

  return (
    <div className="flex h-full bg-slate-950">
      {/* Sidebar */}
      <div className="w-80 flex-shrink-0 bg-slate-900/50 border-r border-slate-800 flex flex-col">
        {/* Header */}
        <div className="p-4 border-b border-slate-800">
          <h2 className="text-lg font-semibold text-slate-100 flex items-center gap-2">
            <Monitor className="w-5 h-5 text-blue-400" />
            Log Viewer
          </h2>
          <p className="text-xs text-slate-500 mt-1">
            Real-time container logs
          </p>
        </div>

        {/* Filters */}
        <div className="flex-1 overflow-y-auto">
          {/* Host Dropdown */}
          <div className="p-4 border-b border-slate-800">
            <label className="text-xs text-slate-500 uppercase tracking-wider mb-2 block">
              Host
            </label>
            <div className="relative">
              <button
                onClick={() => {
                  setHostDropdownOpen(!hostDropdownOpen);
                  setStackDropdownOpen(false);
                  setContainerDropdownOpen(false);
                }}
                className="w-full px-3 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg text-sm flex items-center justify-between transition-colors"
              >
                <div className="flex items-center gap-2">
                  <Server className="w-4 h-4" />
                  <span>{selectedHost === "all" ? "All Hosts" : selectedHost}</span>
                </div>
                <ChevronDown className="w-4 h-4" />
              </button>
              
              {hostDropdownOpen && (
                <>
                  <div
                    className="fixed inset-0 z-40"
                    onClick={() => setHostDropdownOpen(false)}
                  />
                  <div className="absolute top-full left-0 right-0 mt-1 bg-slate-900 border border-slate-800 rounded-lg shadow-xl z-50 max-h-64 overflow-y-auto">
                    <button
                      onClick={() => {
                        setSelectedHost("all");
                        setHostDropdownOpen(false);
                      }}
                      className={cn(
                        "w-full text-left px-3 py-2 text-sm hover:bg-slate-800 transition-colors",
                        selectedHost === "all" ? "bg-slate-800 text-blue-400" : "text-slate-300"
                      )}
                    >
                      All Hosts
                    </button>
                    {logSources.hosts.map((host) => (
                      <button
                        key={host}
                        onClick={() => {
                          setSelectedHost(host);
                          setHostDropdownOpen(false);
                        }}
                        className={cn(
                          "w-full text-left px-3 py-2 text-sm hover:bg-slate-800 transition-colors flex items-center justify-between",
                          selectedHost === host ? "bg-slate-800 text-blue-400" : "text-slate-300"
                        )}
                      >
                        <span>{host}</span>
                        {stats.byHost[host] && (
                          <span className="text-xs text-slate-500">
                            {stats.byHost[host]}
                          </span>
                        )}
                      </button>
                    ))}
                  </div>
                </>
              )}
            </div>
          </div>

          {/* Stack Dropdown */}
          <div className="p-4 border-b border-slate-800">
            <label className="text-xs text-slate-500 uppercase tracking-wider mb-2 block">
              Stack
            </label>
            <div className="relative">
              <button
                onClick={() => {
                  setStackDropdownOpen(!stackDropdownOpen);
                  setHostDropdownOpen(false);
                  setContainerDropdownOpen(false);
                }}
                className={cn(
                  "w-full px-3 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg text-sm flex items-center justify-between transition-colors",
                  filteredStacks.length === 0 && "opacity-50 cursor-not-allowed"
                )}
                disabled={filteredStacks.length === 0}
              >
                <div className="flex items-center gap-2">
                  <Layers className="w-4 h-4" />
                  <span>
                    {selectedStack === "all" 
                      ? "All Stacks" 
                      : selectedStack}
                  </span>
                </div>
                <ChevronDown className="w-4 h-4" />
              </button>
              
              {stackDropdownOpen && filteredStacks.length > 0 && (
                <>
                  <div
                    className="fixed inset-0 z-40"
                    onClick={() => setStackDropdownOpen(false)}
                  />
                  <div className="absolute top-full left-0 right-0 mt-1 bg-slate-900 border border-slate-800 rounded-lg shadow-xl z-50 max-h-64 overflow-y-auto">
                    <button
                      onClick={() => {
                        setSelectedStack("all");
                        setStackDropdownOpen(false);
                      }}
                      className={cn(
                        "w-full text-left px-3 py-2 text-sm hover:bg-slate-800 transition-colors",
                        selectedStack === "all" ? "bg-slate-800 text-blue-400" : "text-slate-300"
                      )}
                    >
                      All Stacks
                    </button>
                    {filteredStacks.map((stack) => (
                      <button
                        key={stack}
                        onClick={() => {
                          setSelectedStack(stack);
                          setStackDropdownOpen(false);
                        }}
                        className={cn(
                          "w-full text-left px-3 py-2 text-sm hover:bg-slate-800 transition-colors flex items-center justify-between",
                          selectedStack === stack ? "bg-slate-800 text-blue-400" : "text-slate-300"
                        )}
                      >
                        <span>{stack}</span>
                        {stats.byStack[stack] && (
                          <span className="text-xs text-slate-500">
                            {stats.byStack[stack]}
                          </span>
                        )}
                      </button>
                    ))}
                  </div>
                </>
              )}
            </div>
          </div>

          {/* Container Dropdown */}
          <div className="p-4 border-b border-slate-800">
            <label className="text-xs text-slate-500 uppercase tracking-wider mb-2 block">
              Container
            </label>
            <div className="relative">
              <button
                onClick={() => {
                  setContainerDropdownOpen(!containerDropdownOpen);
                  setHostDropdownOpen(false);
                  setStackDropdownOpen(false);
                }}
                className={cn(
                  "w-full px-3 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg text-sm flex items-center justify-between transition-colors",
                  filteredContainers.length === 0 && "opacity-50 cursor-not-allowed"
                )}
                disabled={filteredContainers.length === 0}
              >
                <div className="flex items-center gap-2">
                  <Box className="w-4 h-4" />
                  <span className="truncate">
                    {selectedContainer === "all" 
                      ? "All Containers" 
                      : selectedContainer}
                  </span>
                </div>
                <ChevronDown className="w-4 h-4 flex-shrink-0" />
              </button>
              
              {containerDropdownOpen && filteredContainers.length > 0 && (
                <>
                  <div
                    className="fixed inset-0 z-40"
                    onClick={() => setContainerDropdownOpen(false)}
                  />
                  <div className="absolute top-full left-0 right-0 mt-1 bg-slate-900 border border-slate-800 rounded-lg shadow-xl z-50 max-h-64 overflow-y-auto">
                    <button
                      onClick={() => {
                        setSelectedContainer("all");
                        setContainerDropdownOpen(false);
                      }}
                      className={cn(
                        "w-full text-left px-3 py-2 text-sm hover:bg-slate-800 transition-colors",
                        selectedContainer === "all" ? "bg-slate-800 text-blue-400" : "text-slate-300"
                      )}
                    >
                      All Containers
                    </button>
                    {filteredContainers.map((container) => (
                      <button
                        key={`${container.host}-${container.name}`}
                        onClick={() => {
                          setSelectedContainer(container.name);
                          setContainerDropdownOpen(false);
                        }}
                        className={cn(
                          "w-full text-left px-3 py-2 text-sm hover:bg-slate-800 transition-colors",
                          selectedContainer === container.name ? "bg-slate-800 text-blue-400" : "text-slate-300"
                        )}
                      >
                        <div>
                          <div className="font-medium">{container.name}</div>
                          <div className="text-xs text-slate-500">
                            {container.host}
                            {container.stack && ` • ${container.stack}`}
                          </div>
                        </div>
                      </button>
                    ))}
                  </div>
                </>
              )}
            </div>
          </div>

          {/* Log Levels */}
          <div className="p-4 border-b border-slate-800">
            <label className="text-xs text-slate-500 uppercase tracking-wider mb-2 block">
              Log Levels
            </label>
            <div className="space-y-2">
              {["ERROR", "WARN", "INFO", "DEBUG"].map((level) => (
                <label
                  key={level}
                  className="flex items-center gap-2 text-sm cursor-pointer"
                >
                  <input
                    type="checkbox"
                    checked={logLevels.includes(level)}
                    onChange={(e) => {
                      if (e.target.checked) {
                        setLogLevels([...logLevels, level]);
                      } else {
                        setLogLevels(logLevels.filter((l) => l !== level));
                      }
                    }}
                    className="rounded border-slate-600 bg-slate-800 text-blue-500"
                  />
                  <span
                    className={cn(
                      "flex items-center gap-1",
                      level === "ERROR" && "text-red-400",
                      level === "WARN" && "text-yellow-400",
                      level === "INFO" && "text-green-400",
                      level === "DEBUG" && "text-blue-400"
                    )}
                  >
                    {level === "ERROR" && <XCircle className="w-3 h-3" />}
                    {level === "WARN" && <AlertTriangle className="w-3 h-3" />}
                    {level === "INFO" && <Info className="w-3 h-3" />}
                    {level === "DEBUG" && <AlertCircle className="w-3 h-3" />}
                    {level}
                    {stats.byLevel[level] && (
                      <span className="text-slate-500 ml-auto">
                        ({stats.byLevel[level]})
                      </span>
                    )}
                  </span>
                </label>
              ))}
            </div>
          </div>

          {/* Search */}
          <div className="p-4">
            <label className="text-xs text-slate-500 uppercase tracking-wider mb-2 block">
              Search
            </label>
            <input
              type="text"
              placeholder="Filter logs..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full px-3 py-2 bg-slate-800 border border-slate-700 rounded-lg text-sm text-slate-100 placeholder-slate-500 focus:outline-none focus:border-blue-500"
            />
          </div>
        </div>

        {/* Footer Stats */}
        <div className="p-4 border-t border-slate-800 bg-slate-900/50">
          <div className="flex items-center justify-between text-xs text-slate-400">
            <span>Total: {stats.total}</span>
            <span
              className={cn(
                "flex items-center gap-1",
                isConnected ? "text-green-400" : "text-red-400"
              )}
            >
              <div
                className={cn(
                  "w-2 h-2 rounded-full",
                  isConnected ? "bg-green-400" : "bg-red-400"
                )}
              />
              {isConnected ? "Connected" : "Disconnected"}
            </span>
          </div>
        </div>
      </div>

      {/* Main Content */}
      <div className="flex-1 flex flex-col">
        {/* Toolbar */}
        <div className="flex items-center justify-between p-3 border-b border-slate-800 bg-slate-900/50">
          <div className="flex items-center gap-2">
            <button
              onClick={() => setIsFollowing(!isFollowing)}
              className={cn(
                "px-3 py-1.5 rounded-lg text-sm flex items-center gap-2 transition-colors",
                isFollowing
                  ? "bg-green-600 hover:bg-green-700 text-white"
                  : "bg-slate-700 hover:bg-slate-600 text-slate-200"
              )}
            >
              {isFollowing ? (
                <>
                  <Pause className="w-4 h-4" />
                  Pause
                </>
              ) : (
                <>
                  <Play className="w-4 h-4" />
                  Resume
                </>
              )}
            </button>

            <button
              onClick={clearTerminal}
              className="px-3 py-1.5 bg-slate-700 hover:bg-slate-600 text-slate-200 rounded-lg text-sm flex items-center gap-2 transition-colors"
              title={`Clear ${filteredLogs.length} visible logs`}
            >
              <Trash2 className="w-4 h-4" />
              Clear
              {filteredLogs.length > 0 && (
                <span className="text-xs opacity-75">({filteredLogs.length})</span>
              )}
            </button>

            <button
              onClick={exportLogs}
              className={cn(
                "px-3 py-1.5 rounded-lg text-sm flex items-center gap-2 transition-colors",
                filteredLogs.length === 0
                  ? "bg-slate-800 text-slate-500 cursor-not-allowed"
                  : "bg-slate-700 hover:bg-slate-600 text-slate-200"
              )}
              disabled={filteredLogs.length === 0}
              title={filteredLogs.length === 0 ? "No visible logs to export" : `Export ${filteredLogs.length} visible logs`}
            >
              <Download className="w-4 h-4" />
              Export
              {filteredLogs.length > 0 && (
                <span className="text-xs opacity-75">({filteredLogs.length})</span>
              )}
            </button>
          </div>

          <div className="flex items-center gap-2">
            {searchOpen && (
              <input
                type="text"
                placeholder="Search in logs..."
                value={searchTerm}
                onChange={(e) => {
                  setSearchTerm(e.target.value);
                  handleSearch(e.target.value);
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    handleSearch(searchTerm);
                  } else if (e.key === "Escape") {
                    setSearchOpen(false);
                    setSearchTerm("");
                  }
                }}
                className="px-3 py-1.5 bg-slate-800 border border-slate-700 rounded-lg text-sm text-slate-100 placeholder-slate-500 focus:outline-none focus:border-blue-500"
                autoFocus
              />
            )}
            <button
              onClick={() => setSearchOpen(!searchOpen)}
              className="px-3 py-1.5 bg-slate-700 hover:bg-slate-600 text-slate-200 rounded-lg text-sm flex items-center gap-2 transition-colors"
            >
              <Search className="w-4 h-4" />
            </button>
          </div>
        </div>

        {/* Terminal */}
        <div className="flex-1 bg-slate-950 p-2">
          <div ref={terminalRef} className="h-full" />
        </div>
      </div>
    </div>
  );
}