import React, { useState, useEffect } from 'react';
import { GitBranch } from 'lucide-react';
import { debugLog, errorLog } from '@/utils/logging';
import { handle401 } from '@/utils/auth';

interface GitSyncStatus {
  sync_enabled: boolean;
  repo_url: string;
}

export default function GitSyncToggle() {
  const [syncEnabled, setSyncEnabled] = useState(false);
  const [hasRepo, setHasRepo] = useState(false);
  const [isUpdating, setIsUpdating] = useState(false);

  useEffect(() => {
    fetchSyncStatus();
    // Refresh status every 30 seconds
    const interval = setInterval(fetchSyncStatus, 30000);
    return () => clearInterval(interval);
  }, []);

  const fetchSyncStatus = async () => {
    try {
      const response = await fetch('/api/git/config', {
        credentials: 'include',
      });
      if (response.status === 401) {
        handle401();
        return;
      }
      if (response.ok) {
        const data = await response.json();
        setSyncEnabled(data.sync_enabled || false);
        setHasRepo(!!data.repo_url);
      }
    } catch (error) {
      debugLog('Failed to fetch git sync status:', error);
    }
  };

  const handleToggle = async () => {
    if (!hasRepo) {
      // If no repo configured, navigate to git sync page
      window.location.href = '/git';
      return;
    }

    setIsUpdating(true);
    try {
      // First fetch current config
      const configResponse = await fetch('/api/git/config', {
        credentials: 'include',
      });
      
      if (configResponse.status === 401) {
        handle401();
        return;
      }
      
      if (!configResponse.ok) {
        throw new Error('Failed to fetch config');
      }
      
      const currentConfig = await configResponse.json();
      
      // Build update payload - only send necessary fields
      // Note: has_token and has_ssh_key are response-only fields
      const updatedConfig = {
        repo_url: currentConfig.repo_url || '',
        branch: currentConfig.branch || 'main',
        commit_author_name: currentConfig.commit_author_name || 'DD-UI Bot',
        commit_author_email: currentConfig.commit_author_email || 'ddui@localhost',
        sync_enabled: !syncEnabled,
        auto_push: currentConfig.auto_push || false,
        auto_pull: currentConfig.auto_pull || false,
        pull_interval_mins: currentConfig.pull_interval_mins || 5,
        push_on_change: currentConfig.push_on_change || false,
        sync_path: currentConfig.sync_path || '/data',
        // Don't send token/key fields - backend will preserve them
      };
      
      // Update config
      const response = await fetch('/api/git/config', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(updatedConfig),
      });
      
      if (response.status === 401) {
        handle401();
        return;
      }
      
      if (response.ok) {
        setSyncEnabled(!syncEnabled);
        debugLog(`Git sync ${!syncEnabled ? 'enabled' : 'disabled'}`);
        // Refetch to ensure we have the latest state
        await fetchSyncStatus();
      } else {
        throw new Error('Failed to update sync status');
      }
    } catch (error) {
      errorLog('Failed to toggle git sync:', error);
      // Refetch status in case of error to reset to correct state
      await fetchSyncStatus();
    } finally {
      setIsUpdating(false);
    }
  };

  // Determine color based on state
  const getToggleColor = () => {
    if (!hasRepo) return 'text-slate-500 hover:text-slate-400';
    if (isUpdating) return 'text-slate-400';
    if (syncEnabled) return 'text-green-400 hover:text-green-300';
    return 'text-slate-400 hover:text-slate-300';
  };

  const getTooltip = () => {
    if (!hasRepo) return 'Configure Git repository';
    if (syncEnabled) return 'Git sync enabled - Click to disable';
    return 'Git sync disabled - Click to enable';
  };

  // Always render the button, even if loading
  return (
    <button
      onClick={handleToggle}
      disabled={isUpdating}
      className={`flex items-center gap-1.5 px-2 py-1 rounded-md transition-colors ${getToggleColor()} text-xs font-medium`}
      title={getTooltip()}
      style={{ minWidth: '70px' }} // Ensure minimum width so it doesn't disappear
    >
      <GitBranch className={`w-3.5 h-3.5 ${isUpdating ? 'animate-pulse' : ''}`} />
      <span className="hidden sm:inline">
        {!hasRepo ? 'Setup' : syncEnabled ? 'Synced' : 'Not Synced'}
      </span>
      {/* Tiny indicator dot for mobile */}
      <span className={`sm:hidden w-1.5 h-1.5 rounded-full ${
        !hasRepo ? 'bg-slate-500' : syncEnabled ? 'bg-green-400' : 'bg-slate-400'
      }`} />
    </button>
  );
}