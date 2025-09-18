import React, { useState, useEffect } from 'react';
import TriStateToggle from './TriStateToggle';
import { debugLog, errorLog } from '@/utils/logging';
import { handle401 } from '@/utils/auth';

interface DevOpsToggleProps {
  level?: 'global' | 'host' | 'stack';
  hostName?: string;
  stackName?: string;
  groupName?: string;
  className?: string;
  compact?: boolean;
}

export default function DevOpsToggle({ 
  level = 'global', 
  hostName, 
  stackName,
  groupName,
  className = '',
  compact = false
}: DevOpsToggleProps) {
  const [override, setOverride] = useState<boolean | null>(null);
  const [effective, setEffective] = useState<boolean>(false);
  const [inheritsFrom, setInheritsFrom] = useState<string>('');
  const [isUpdating, setIsUpdating] = useState(false);

  useEffect(() => {
    fetchDevOpsStatus();
    // Refresh status every 30 seconds
    const interval = setInterval(fetchDevOpsStatus, 30000);
    return () => clearInterval(interval);
  }, [level, hostName, stackName, groupName]);

  const getEndpoint = () => {
    // Stack-level endpoints
    if (level === 'stack' && hostName && stackName) {
      return `/api/devops/hosts/${encodeURIComponent(hostName)}/stacks/${encodeURIComponent(stackName)}`;
    }
    if (level === 'stack' && groupName && stackName) {
      return `/api/devops/groups/${encodeURIComponent(groupName)}/stacks/${encodeURIComponent(stackName)}`;
    }
    
    // Host-level endpoint
    if (level === 'host' && hostName) {
      return `/api/devops/hosts/${encodeURIComponent(hostName)}`;
    }
    
    // Group-level endpoint  
    if (level === 'host' && groupName) {
      return `/api/devops/groups/${encodeURIComponent(groupName)}`;
    }
    
    // Global endpoint
    return '/api/devops/global';
  };

  const fetchDevOpsStatus = async () => {
    try {
      const response = await fetch(getEndpoint(), {
        credentials: 'include',
      });
      if (response.status === 401) {
        handle401();
        return;
      }
      if (response.ok) {
        const data = await response.json();
        
        // Set override state (null means inheriting)
        setOverride(data.override !== undefined ? data.override : null);
        
        // Set effective value (what's actually being used)
        setEffective(data.effective !== undefined ? data.effective : data.auto_deploy || false);
        
        // Set where it inherits from (if applicable)
        if (data.override === null || data.override === undefined) {
          setInheritsFrom(data.inherits_from || data.source || '');
        } else {
          setInheritsFrom('');  // Has explicit override, not inheriting
        }
      }
    } catch (error) {
      debugLog('Failed to fetch DevOps status:', error);
    }
  };

  const handleChange = async (newValue: boolean | null) => {
    setIsUpdating(true);
    try {
      const response = await fetch(getEndpoint(), {
        method: 'PATCH',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ auto_deploy: newValue }),
      });
      
      if (response.status === 401) {
        handle401();
        return;
      }
      
      if (response.ok) {
        const data = await response.json();
        setOverride(data.override !== undefined ? data.override : null);
        setEffective(data.effective !== undefined ? data.effective : data.auto_deploy || false);
        
        if (data.override === null || data.override === undefined) {
          setInheritsFrom(data.inherits_from || data.source || '');
        } else {
          setInheritsFrom('');
        }
        
        const action = newValue === null ? 'unset' : newValue ? 'enabled' : 'disabled';
        debugLog(`DevOps ${action} at ${level} level`);
      } else {
        throw new Error('Failed to update DevOps status');
      }
    } catch (error) {
      errorLog('Failed to toggle DevOps:', error);
      // Refetch status in case of error to reset to correct state
      await fetchDevOpsStatus();
    } finally {
      setIsUpdating(false);
    }
  };

  // For display: if there's an override, show that; otherwise show effective with inheritance indicator
  const displayValue = override !== null ? override : effective;
  const showInheritance = override === null && inheritsFrom;

  // Determine scope label
  const getScopeLabel = () => {
    if (level === 'global') return 'global';
    if (level === 'host') return hostName ? 'host' : groupName ? 'group' : 'unknown';
    if (level === 'stack') return 'stack';
    return level;
  };

  return (
    <TriStateToggle
      value={override}
      onChange={handleChange}
      label="DevOps"
      disabled={isUpdating}
      inheritedFrom={showInheritance ? inheritsFrom : undefined}
      scope={getScopeLabel()}
      className={className}
      compact={compact}
    />
  );
}