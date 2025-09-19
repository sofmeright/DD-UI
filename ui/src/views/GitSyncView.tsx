import React, { useState, useEffect } from 'react';
import { GitBranch, AlertCircle, Check, X, RefreshCw, Upload, Download, Settings, AlertTriangle, GitCommit } from 'lucide-react';
import { debugLog, errorLog } from '@/utils/logging';
import { handle401 } from '@/utils/auth';

interface GitConfig {
  repo_url: string;
  branch: string;
  has_token: boolean;
  has_ssh_key: boolean;
  auth_token?: string;
  ssh_key?: string;
  commit_author_name: string;
  commit_author_email: string;
  sync_enabled: boolean;
  sync_mode?: string;  // 'off' | 'push' | 'pull' | 'sync'
  force_on_conflict?: boolean;
  auto_push: boolean;
  auto_pull: boolean;
  pull_interval_mins: number;
  push_on_change: boolean;
  sync_path?: string;  // Path from DD_UI_IAC_ROOT
}

interface GitStatus {
  sync_enabled: boolean;
  running: boolean;
  last_pull_at?: string;
  last_push_at?: string;
  last_commit?: string;
  last_status?: string;
  last_message?: string;
  last_error?: string;
  unresolved_conflicts?: number;
}

export default function GitSyncView() {
  const [config, setConfig] = useState<GitConfig | null>(null);
  const [status, setStatus] = useState<GitStatus | null>(null);
  const [isSaving, setIsSaving] = useState(false);
  const [isTestingConnection, setIsTestingConnection] = useState(false);
  const [testResult, setTestResult] = useState<{ success: boolean; message: string } | null>(null);
  const [isSyncing, setIsSyncing] = useState(false);
  const [isPulling, setIsPulling] = useState(false);
  const [isPushing, setIsPushing] = useState(false);
  const [commitMessage, setCommitMessage] = useState('');
  const [editedConfig, setEditedConfig] = useState<Partial<GitConfig>>({});
  // Store credentials separately so they persist across edit sessions
  const [savedCredentials, setSavedCredentials] = useState<{ token: string; key: string }>({ token: '', key: '' });
  const [loadError, setLoadError] = useState<string | null>(null);
  const [syncNotification, setSyncNotification] = useState<{ type: 'success' | 'error' | 'info'; message: string; operation?: string } | null>(null);
  const [iacRoot, setIacRoot] = useState<string>('/data'); // Default to /data if not provided
  const [devOpsEnabled, setDevOpsEnabled] = useState<boolean>(false);
  const [isTogglingDevOps, setIsTogglingDevOps] = useState(false);

  useEffect(() => {
    fetchConfig();
    fetchStatus();
    fetchDevOpsStatus();
    
    // Refresh status every 10 seconds
    const interval = setInterval(() => {
      fetchStatus();
    }, 10000);
    
    return () => clearInterval(interval);
  }, []);

  const fetchConfig = async (preserveCredentials = false) => {
    try {
      // Add timeout to prevent hanging
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 5000); // 5 second timeout
      
      const response = await fetch('/api/git/config', {
        credentials: 'include',
        signal: controller.signal,
      });
      
      clearTimeout(timeoutId);
      debugLog('Git config response status:', response.status);
      if (response.ok) {
        const data = await response.json();
        debugLog('Git config received:', data);
        setConfig(data);
        
        // Set the IAC root from the sync_path if available
        if (data.sync_path) {
          setIacRoot(data.sync_path);
        }
        
        // When preserving credentials, keep the actual credential values
        // but update all other fields from the server response
        if (preserveCredentials) {
          setEditedConfig(prev => {
            const updated = { ...data,
              // Keep the actual credential values if they exist (not the boolean flags)
              auth_token: (typeof prev.auth_token === 'string' ? prev.auth_token : '') || '',
              ssh_key: (typeof prev.ssh_key === 'string' ? prev.ssh_key : '') || '',
            };
            debugLog('Updated editedConfig after save:', {  ...updated, 
              auth_token: updated.auth_token ? '***' : '', 
              ssh_key: updated.ssh_key ? '***' : '' 
            });
            return updated;
          });
        } else {
          // When not preserving (initial load), use saved credentials if available
          setEditedConfig({ ...data,
            auth_token: savedCredentials.token || '',
            ssh_key: savedCredentials.key || '',
          });
        }
      } else {
        // If no config exists yet, set a default one
        const defaultConfig: GitConfig = {
          repo_url: '',
          branch: 'main',
          has_token: false,
          has_ssh_key: false,
          commit_author_name: 'DD-UI Bot',
          commit_author_email: 'ddui@localhost',
          sync_enabled: false,
          auto_push: false,
          auto_pull: false,
          pull_interval_mins: 5,
          push_on_change: true,
        };
        setConfig(defaultConfig);
        setEditedConfig(defaultConfig);
      }
    } catch (error: any) {
      if (error.name === 'AbortError') {
        errorLog('Git config fetch timed out');
        setLoadError('Request timed out loading Git configuration');
      } else {
        errorLog('Failed to fetch git config:', error);
        setLoadError('Failed to load Git configuration');
      }
      // Set a default config even on error
      const defaultConfig: GitConfig = {
        repo_url: '',
        branch: 'main',
        has_token: false,
        has_ssh_key: false,
        commit_author_name: 'DD-UI Bot',
        commit_author_email: 'ddui@localhost',
        sync_enabled: false,
        auto_push: false,
        auto_pull: false,
        pull_interval_mins: 5,
        push_on_change: true,
      };
      setConfig(defaultConfig);
      setEditedConfig(defaultConfig);
    }
  };

  const fetchDevOpsStatus = async () => {
    try {
      const response = await fetch('/api/devops/global', {
        credentials: 'include',
      });
      if (response.ok) {
        const data = await response.json();
        setDevOpsEnabled(data.auto_deploy || false);
      }
    } catch (error) {
      errorLog('Failed to fetch DevOps status:', error);
    }
  };

  const toggleDevOps = async (enabled: boolean) => {
    setIsTogglingDevOps(true);
    try {
      const response = await fetch('/api/devops/global', {
        method: 'PATCH',
        headers: {
          'Content-Type': 'application/json',
        },
        credentials: 'include',
        body: JSON.stringify({ auto_deploy: enabled }),
      });
      if (response.ok) {
        const data = await response.json();
        setDevOpsEnabled(data.auto_deploy || false);
        setSyncNotification({
          type: 'success',
          message: `DevOps ${enabled ? 'enabled' : 'disabled'} successfully`,
        });
      } else {
        setSyncNotification({
          type: 'error',
          message: 'Failed to update DevOps setting',
        });
      }
    } catch (error) {
      errorLog('Failed to toggle DevOps:', error);
      setSyncNotification({
        type: 'error',
        message: 'Failed to update DevOps setting',
      });
    } finally {
      setIsTogglingDevOps(false);
    }
  };

  const fetchStatus = async () => {
    try {
      // Add timeout to prevent hanging
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 5000); // 5 second timeout
      
      const response = await fetch('/api/git/status', {
        credentials: 'include',
        signal: controller.signal,
      });
      
      clearTimeout(timeoutId);
      if (response.ok) {
        const data = await response.json();
        setStatus(data);
      }
    } catch (error: any) {
      if (error.name === 'AbortError') {
        errorLog('Git status fetch timed out');
      } else {
        errorLog('Failed to fetch git status:', error);
      }
    }
  };


  const handleSaveConfig = async () => {
    setIsSaving(true);
    try {
      // Store credentials separately to preserve them
      const currentToken = editedConfig.auth_token;
      const currentKey = editedConfig.ssh_key;
      
      debugLog('Saving config with credentials:', {
        hasToken: !!currentToken,
        hasKey: !!currentKey,
        keyLength: currentKey ? currentKey.length : 0
      });
      
      // Build a clean config object with only the fields the backend expects
      debugLog('Full editedConfig state:', editedConfig);
      debugLog('editedConfig specific fields:', {
        repo_url: editedConfig.repo_url,
        branch: editedConfig.branch,
        author_name: editedConfig.commit_author_name,
        author_email: editedConfig.commit_author_email,
        sync_path: editedConfig.sync_path,
      });
      
      const configToSave = {
        repo_url: editedConfig.repo_url || '',
        branch: editedConfig.branch || 'main',
        commit_author_name: editedConfig.commit_author_name || 'DD-UI Bot',
        commit_author_email: editedConfig.commit_author_email || 'ddui@localhost',
        sync_enabled: editedConfig.sync_enabled || false,
        sync_mode: editedConfig.sync_mode || 'off',
        force_on_conflict: editedConfig.force_on_conflict || false,
        auto_push: editedConfig.auto_push || false,
        auto_pull: editedConfig.auto_pull || false,
        pull_interval_mins: editedConfig.pull_interval_mins || 5,
        push_on_change: true, // Always enabled by default
        // sync_path is now derived from DD_UI_IAC_ROOT on backend
        // Only include auth fields if they have values (not the boolean flags) ...(currentToken && typeof currentToken === 'string' && { auth_token: currentToken }), ...(currentKey && typeof currentKey === 'string' && { ssh_key: currentKey }),
      };
      
      debugLog('Config being sent to server:', {  ...configToSave, 
        auth_token: currentToken ? '***' : '', 
        ssh_key: currentKey ? '***' : '' 
      });
      
      const response = await fetch('/api/git/config', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(configToSave),
      });
      
      if (response.ok) {
        debugLog('Git config saved successfully, updating local state');
        
        // Save the credentials for future use
        setSavedCredentials({
          token: currentToken || '',
          key: currentKey || ''
        });
        
        // Update the config locally with the saved values
        // We don't need to fetch from server since we know what we saved
        setConfig(prev => ({ ...prev, ...configToSave,
          has_token: !!currentToken,
          has_ssh_key: !!currentKey,
        }));
        
        // Keep the credentials in editedConfig
        setEditedConfig(prev => ({ ...configToSave,
          auth_token: currentToken || '',
          ssh_key: currentKey || '',
          has_token: !!currentToken,
          has_ssh_key: !!currentKey,
        }));
        
        // Fetch status in background (don't await)
        fetchStatus().catch(error => {
          errorLog('Failed to fetch status after save:', error);
        });
        
        setIsSaving(false);
        debugLog('Configuration saved successfully');
      } else {
        const errorText = await response.text();
        throw new Error(`Failed to save configuration: ${errorText}`);
      }
    } catch (error) {
      errorLog('Failed to save git config:', error);
      alert(`Failed to save configuration: ${error.message}`);
    } finally {
      setIsSaving(false);
    }
  };

  const handleTestConnection = async () => {
    setIsTestingConnection(true);
    setTestResult(null);
    
    try {
      // Only send actual credential strings, not boolean flags
      const testConfig = {
        repo_url: editedConfig.repo_url,
        branch: editedConfig.branch || 'main',
      };
      
      // Only add credentials if they're actual strings
      if (editedConfig.auth_token && typeof editedConfig.auth_token === 'string') {
        testConfig['auth_token'] = editedConfig.auth_token;
      }
      if (editedConfig.ssh_key && typeof editedConfig.ssh_key === 'string') {
        testConfig['ssh_key'] = editedConfig.ssh_key;
      }
      
      debugLog('Testing connection with config:', { ...testConfig, auth_token: testConfig.auth_token ? '***' : '', ssh_key: testConfig.ssh_key ? '***' : '' });
      
      const response = await fetch('/api/git/test', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(testConfig),
      });
      
      const data = await response.json();
      setTestResult({
        success: response.ok,
        message: data.message,
      });
    } catch (error) {
      setTestResult({
        success: false,
        message: 'Connection test failed: ' + error.message,
      });
    } finally {
      setIsTestingConnection(false);
    }
  };

  const handleSync = async () => {
    setIsSyncing(true);
    setSyncNotification({ type: 'info', message: 'Syncing with remote repository...', operation: 'sync' });
    try {
      const response = await fetch('/api/git/sync', {
        method: 'POST',
        credentials: 'include',
      });
      
      if (response.status === 401) {
        handle401();
        return;
      }
      
      if (response.ok) {
        const result = await response.json();
        await fetchStatus();
        setSyncNotification({ 
          type: 'success', 
          message: result.message || 'Repository synchronized successfully',
          operation: 'sync'
        });
        debugLog('Git sync completed');
        // Clear notification after 5 seconds
        setTimeout(() => setSyncNotification(null), 5000);
      } else {
        const error = await response.json();
        throw new Error(error.message || 'Sync failed');
      }
    } catch (error: any) {
      errorLog('Git sync failed:', error);
      setSyncNotification({ 
        type: 'error', 
        message: error.message || 'Failed to sync with remote repository',
        operation: 'sync'
      });
      setTimeout(() => setSyncNotification(null), 8000);
    } finally {
      setIsSyncing(false);
    }
  };

  const handlePull = async () => {
    setIsPulling(true);
    setSyncNotification({ type: 'info', message: 'Pulling changes from remote...', operation: 'pull' });
    try {
      const response = await fetch('/api/git/pull', {
        method: 'POST',
        credentials: 'include',
      });
      
      if (response.status === 401) {
        handle401();
        return;
      }
      
      if (response.ok) {
        const result = await response.json();
        await fetchStatus();
        setSyncNotification({ 
          type: 'success', 
          message: result.message || 'Successfully pulled latest changes',
          operation: 'pull'
        });
        debugLog('Git pull completed');
        setTimeout(() => setSyncNotification(null), 5000);
      } else {
        const error = await response.json();
        throw new Error(error.message || 'Pull failed');
      }
    } catch (error: any) {
      errorLog('Git pull failed:', error);
      setSyncNotification({ 
        type: 'error', 
        message: error.message || 'Failed to pull from remote repository',
        operation: 'pull'
      });
      setTimeout(() => setSyncNotification(null), 8000);
    } finally {
      setIsPulling(false);
    }
  };

  const handlePush = async () => {
    debugLog('Push button clicked, starting push...');
    setIsPushing(true);
    setLoadError(null); // Clear any previous errors
    
    try {
      debugLog('Sending push request with message:', commitMessage || '(empty)');
      
      // Add timeout to prevent hanging
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 30000); // 30 second timeout
      
      const response = await fetch('/api/git/push', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: commitMessage || 'Manual push from DD-UI' }),
        signal: controller.signal,
      }).finally(() => clearTimeout(timeoutId));
      
      debugLog('Push response status:', response.status);
      
      if (response.status === 401) {
        handle401();
        return;
      }
      
      if (response.ok) {
        const result = await response.json();
        debugLog('Push succeeded:', result);
        await fetchStatus();
        setCommitMessage('');
        setSyncNotification({ 
          type: 'success', 
          message: result.message || 'Successfully pushed changes to remote',
          operation: 'push'
        });
        setTimeout(() => setSyncNotification(null), 5000);
        setLoadError(null);
      } else {
        const errorText = await response.text();
        let errorMessage = 'Push failed';
        try {
          const errorJson = JSON.parse(errorText);
          errorMessage = errorJson.message || errorJson.error || errorText;
        } catch {
          errorMessage = errorText || 'Push failed with no error message';
        }
        errorLog('Push failed with status', response.status, ':', errorMessage);
        setSyncNotification({ 
          type: 'error', 
          message: errorMessage,
          operation: 'push'
        });
        setTimeout(() => setSyncNotification(null), 8000);
        setLoadError(`Push failed: ${errorMessage}`);
      }
    } catch (error: any) {
      if (error.name === 'AbortError') {
        errorLog('Push request timed out after 30 seconds');
        setSyncNotification({ 
          type: 'error', 
          message: 'Push timed out - the operation is taking too long',
          operation: 'push'
        });
        setLoadError('Push timed out - the operation is taking too long');
      } else {
        errorLog('Git push error:', error);
        setSyncNotification({ 
          type: 'error', 
          message: error.message || 'Failed to push to remote repository',
          operation: 'push'
        });
        setLoadError(`Push error: ${error.message || 'Unknown error'}`);
      }
      setTimeout(() => setSyncNotification(null), 8000);
    } finally {
      setIsPushing(false);
      debugLog('Push operation completed');
    }
  };

  const formatDate = (dateString?: string) => {
    if (!dateString) return 'Never';
    const date = new Date(dateString);
    return date.toLocaleString();
  };

  if (!config) {
    return (
      <div className="flex items-center justify-center h-64">
        <div className="text-center">
          <RefreshCw className="w-8 h-8 animate-spin text-slate-400 mx-auto mb-4" />
          <p className="text-slate-400">Loading Git configuration...</p>
          {loadError && (
            <p className="text-red-400 mt-2">{loadError}</p>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="p-6 max-w-6xl mx-auto">
      {/* Sync Notification */}
      {syncNotification && (
        <div className={`mb-6 p-4 rounded-lg border transition-all duration-300 ${
          syncNotification.type === 'success' ? 'bg-green-900/30 border-green-700 text-green-400' :
          syncNotification.type === 'error' ? 'bg-red-900/30 border-red-700 text-red-400' :
          'bg-blue-900/30 border-blue-700 text-blue-400'
        }`}>
          <div className="flex items-center gap-3">
            {syncNotification.type === 'success' && <Check className="w-5 h-5" />}
            {syncNotification.type === 'error' && <X className="w-5 h-5" />}
            {syncNotification.type === 'info' && <RefreshCw className="w-5 h-5 animate-spin" />}
            <div className="flex-1">
              <p className="font-medium">
                {syncNotification.operation === 'pull' && 'Pull Operation'}
                {syncNotification.operation === 'push' && 'Push Operation'}
                {syncNotification.operation === 'sync' && 'Sync Operation'}
              </p>
              <p className="text-sm opacity-90">{syncNotification.message}</p>
            </div>
          </div>
        </div>
      )}

      {/* Automated Deployment Section */}
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-slate-100">
          Automated Deployment
        </h1>
        <p className="text-slate-400 mt-1">
          Enable DD-UI to deploy stacks/containers automatically when they are explained in infrastructure as code
        </p>
      </div>

      {/* DevOps Toggle */}
      <div className="bg-slate-800 rounded-lg p-4 mb-2">
        <div className="flex items-center justify-between">
          <div>
            <h3 className="text-lg font-semibold text-slate-100">DevOps</h3>
            <p className="text-sm text-slate-400">Automatically deploy changes from {iacRoot}</p>
          </div>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={() => toggleDevOps(false)}
              disabled={isTogglingDevOps}
              className={`px-3 py-1 rounded font-medium transition-colors ${
                !devOpsEnabled ?
                'bg-red-600 text-white' :
                'bg-slate-700 text-slate-400 hover:bg-slate-600'
              } ${isTogglingDevOps ? 'opacity-50 cursor-not-allowed' : ''}`}
            >
              Off
            </button>
            <button
              type="button"
              onClick={() => toggleDevOps(true)}
              disabled={isTogglingDevOps}
              className={`px-3 py-1 rounded font-medium transition-colors ${
                devOpsEnabled ?
                'bg-green-600 text-white' :
                'bg-slate-700 text-slate-400 hover:bg-slate-600'
              } ${isTogglingDevOps ? 'opacity-50 cursor-not-allowed' : ''}`}
            >
              On
            </button>
          </div>
        </div>
      </div>
      
      {/* Note about overrides */}
      <div className="mb-6">
        <p className="text-sm text-slate-500 italic">
          Note: Host & stack level overrides are available on their respective pages
        </p>
      </div>

      <div className="mb-6">
        <h1 className="text-2xl font-bold text-slate-100 flex items-center gap-2">
          <GitBranch className="w-6 h-6" />
          Git Repository Sync
        </h1>
        <p className="text-slate-400 mt-1">
          Synchronize your DD-UI configuration with a Git repository. (inventory/stacks stored in {iacRoot})
        </p>
      </div>

      {/* Status Bar */}
      <div className="bg-slate-800 rounded-lg p-4 mb-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-2">
              {status?.sync_enabled ? (
                <Check className="w-5 h-5 text-green-400" />
              ) : (
                <X className="w-5 h-5 text-red-400" />
              )}
              <span className="text-slate-200">
                Sync {status?.sync_enabled ? 'Enabled' : 'Disabled'}
              </span>
            </div>
            
            {status?.last_commit && (
              <div className="flex items-center gap-2 text-slate-400">
                <GitCommit className="w-4 h-4" />
                <code className="text-xs">{status.last_commit}</code>
              </div>
            )}
            
            {status?.unresolved_conflicts && status.unresolved_conflicts > 0 && (
              <div className="flex items-center gap-2 text-yellow-400">
                <AlertTriangle className="w-4 h-4" />
                <span>{status.unresolved_conflicts} conflicts</span>
              </div>
            )}
          </div>
          
          <div className="flex items-center gap-2">
            <button
              onClick={handlePull}
              disabled={isPulling || !status?.sync_enabled}
              className="px-3 py-1 bg-slate-700 hover:bg-slate-600 text-slate-200 rounded flex items-center gap-2"
            >
              <Download className="w-4 h-4" />
              Pull
            </button>
            
            <button
              onClick={handleSync}
              disabled={isSyncing || !status?.sync_enabled}
              className="px-3 py-1 bg-blue-600 hover:bg-blue-700 text-white rounded flex items-center gap-2"
            >
              <RefreshCw className={`w-4 h-4 ${isSyncing ? 'animate-spin' : ''}`} />
              Sync
            </button>
            
            <button
              onClick={handlePush}
              disabled={isPushing || !status?.sync_enabled}
              className="px-3 py-1 bg-slate-700 hover:bg-slate-600 text-slate-200 rounded flex items-center gap-2"
            >
              <Upload className="w-4 h-4" />
              Push
            </button>
          </div>
        </div>
        
        {status?.last_error && (
          <div className="mt-3 p-2 bg-red-900/30 border border-red-700 rounded text-red-400 text-sm">
            <AlertCircle className="w-4 h-4 inline mr-2" />
            {status.last_error}
          </div>
        )}
      </div>

      {/* Configuration */}
      <div className="bg-slate-800 rounded-lg p-6 mb-6">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-xl font-semibold text-slate-100 flex items-center gap-2">
            <Settings className="w-5 h-5" />
            Repository Configuration
          </h2>
          
          <div className="flex items-center gap-4">
            <label className="text-sm font-medium text-slate-300">Sync</label>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={() => setEditedConfig({ ...editedConfig, sync_enabled: false })}
                className={`px-3 py-1 rounded font-medium transition-colors ${
                  !editedConfig.sync_enabled ?
                  'bg-red-600 text-white' :
                  'bg-slate-700 text-slate-400 hover:bg-slate-600'
                }`}
              >
                Off
              </button>
              <button
                type="button"
                onClick={() => setEditedConfig({ ...editedConfig, sync_enabled: true })}
                className={`px-3 py-1 rounded font-medium transition-colors ${
                  editedConfig.sync_enabled ?
                  'bg-green-600 text-white' :
                  'bg-slate-700 text-slate-400 hover:bg-slate-600'
                }`}
              >
                On
              </button>
            </div>
          </div>
        </div>

        <div className="space-y-4">
          <div className="grid grid-cols-2 gap-4">
            <div>
              <label className="block text-sm font-medium text-slate-300 mb-1">
                Commit Author Name
              </label>
              <input
                type="text"
                value={editedConfig.commit_author_name || ''}
                onChange={(e) => setEditedConfig({ ...editedConfig, commit_author_name: e.target.value })}
                className="w-full px-3 py-2 bg-slate-900 border border-slate-700 rounded text-slate-200"
              />
            </div>

            <div>
              <label className="block text-sm font-medium text-slate-300 mb-1">
                Commit Author Email
              </label>
              <input
                type="email"
                value={editedConfig.commit_author_email || ''}
                onChange={(e) => setEditedConfig({ ...editedConfig, commit_author_email: e.target.value })}
                className="w-full px-3 py-2 bg-slate-900 border border-slate-700 rounded text-slate-200"
              />
            </div>
          </div>

          <div className="flex gap-4">
            <div className="flex-1">
              <label className="block text-sm font-medium text-slate-300 mb-1">
                Repository URL
              </label>
              <input
                type="text"
                value={editedConfig.repo_url || ''}
                onChange={(e) => {
                  const newValue = e.target.value;
                  debugLog('Repository URL changed to:', newValue);
                  setEditedConfig(prev => ({ ...prev, repo_url: newValue }));
                }}
                placeholder="https://github.com/username/repo.git"
                className="w-full px-3 py-2 bg-slate-900 border border-slate-700 rounded text-slate-200"
              />
            </div>
            <div className="w-[30%]">
              <label className="block text-sm font-medium text-slate-300 mb-1">
                Branch
              </label>
              <input
                type="text"
                value={editedConfig.branch || ''}
                onChange={(e) => setEditedConfig({ ...editedConfig, branch: e.target.value })}
                placeholder="main"
                className="w-full px-3 py-2 bg-slate-900 border border-slate-700 rounded text-slate-200"
              />
            </div>
          </div>

          <div>
            <label className="block text-sm font-medium text-slate-300 mb-1">
              Authentication Token (GitHub/GitLab)
            </label>
            <input
              type="password"
              value={(typeof editedConfig.auth_token === 'string' ? editedConfig.auth_token : '') || ''}
              onChange={(e) => setEditedConfig({ ...editedConfig, auth_token: e.target.value })}
              placeholder={config.has_token ? '••••••• (configured)' : 'Enter personal access token'}
              className="w-full px-3 py-2 bg-slate-900 border border-slate-700 rounded text-slate-200"
            />
            <p className="text-xs text-slate-500 mt-1">
              Personal access token for HTTPS authentication
            </p>
          </div>

          <div>
            <label className="block text-sm font-medium text-slate-300 mb-1">
              SSH Private Key (Alternative to Token)
            </label>
            <textarea
              value={(typeof editedConfig.ssh_key === 'string' ? editedConfig.ssh_key : '') || ''}
              onChange={(e) => setEditedConfig({ ...editedConfig, ssh_key: e.target.value })}
              placeholder={config.has_ssh_key ? 'SSH key configured' : 'Paste SSH private key'}
              rows={3}
              className="w-full px-3 py-2 bg-slate-900 border border-slate-700 rounded text-slate-200 font-mono text-xs"
            />
          </div>

          {editedConfig.sync_enabled && (
            <div className="space-y-2">
              <div className="space-y-2">
                <label className="block text-sm font-medium text-slate-300 mb-2">
                  Sync Mode
                </label>
                <div className="grid grid-cols-3 gap-2">
                  <button
                    type="button"
                    onClick={() => setEditedConfig({ 
                      ...editedConfig, 
                      sync_mode: 'pull',
                      auto_pull: true,
                      auto_push: false
                    })}
                    className={`px-3 py-2 rounded-lg font-medium transition-colors ${
                      (editedConfig.sync_mode === 'pull' || (editedConfig.auto_pull && !editedConfig.auto_push)) ?
                      'bg-blue-600 text-white' :
                      'bg-slate-800 text-slate-400 hover:bg-slate-700'
                    }`}
                  >
                    <Download className="w-4 h-4 inline-block mr-1" />
                    Pull
                  </button>
                  
                  <button
                    type="button"
                    onClick={() => setEditedConfig({ 
                      ...editedConfig, 
                      sync_mode: 'sync',
                      auto_pull: true,
                      auto_push: true
                    })}
                    className={`px-3 py-2 rounded-lg font-medium transition-colors ${
                      (editedConfig.sync_mode === 'sync' || (editedConfig.auto_pull && editedConfig.auto_push)) ?
                      'bg-blue-600 text-white' :
                      'bg-slate-800 text-slate-400 hover:bg-slate-700'
                    }`}
                  >
                    <RefreshCw className="w-4 h-4 inline-block mr-1" />
                    Sync
                  </button>
                  
                  <button
                    type="button"
                    onClick={() => setEditedConfig({ 
                      ...editedConfig, 
                      sync_mode: 'push',
                      auto_pull: false,
                      auto_push: true
                    })}
                    className={`px-3 py-2 rounded-lg font-medium transition-colors ${
                      (editedConfig.sync_mode === 'push' || (!editedConfig.auto_pull && editedConfig.auto_push)) ?
                      'bg-blue-600 text-white' :
                      'bg-slate-800 text-slate-400 hover:bg-slate-700'
                    }`}
                  >
                    <Upload className="w-4 h-4 inline-block mr-1" />
                    Push
                  </button>
                </div>
                <p className="text-xs text-slate-500 mt-2">
                  Pull: Continuously pull from remote | Sync: Bidirectional sync | Push: Continuously push to remote
                </p>
              </div>
            </div>
          )}

          <div className="flex items-center justify-between pt-4">
            <button
              onClick={handleTestConnection}
              disabled={isTestingConnection || !editedConfig.repo_url}
              className="px-4 py-2 bg-slate-700 hover:bg-slate-600 text-slate-200 rounded"
            >
              {isTestingConnection ? 'Testing...' : 'Test Connection'}
            </button>
            
            <button
              onClick={handleSaveConfig}
              disabled={isSaving}
              className="px-4 py-2 bg-green-600 hover:bg-green-700 text-white rounded"
            >
              {isSaving ? 'Saving...' : 'Save'}
            </button>
          </div>

          {testResult && (
            <div className={`p-3 rounded ${testResult.success ? 'bg-green-900/30 border border-green-700 text-green-400' : 'bg-red-900/30 border border-red-700 text-red-400'}`}>
              {testResult.success ? (
                <Check className="w-4 h-4 inline mr-2" />
              ) : (
                <X className="w-4 h-4 inline mr-2" />
              )}
              {testResult.message}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}