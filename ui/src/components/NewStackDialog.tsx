import React, { useState, useEffect } from 'react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Boxes, Server, Users } from "lucide-react";
import { infoLog, errorLog } from "@/utils/logging";
import { Host } from "@/types";

interface NewStackDialogProps {
  open: boolean;
  onClose: () => void;
  onStackCreated?: (scopeKind: string, scopeName: string, stackName: string) => void;
  hosts: Host[];
  groups?: string[];
  defaultHost?: string;
  defaultScopeKind?: 'host' | 'group';
}

export default function NewStackDialog({ 
  open, 
  onClose, 
  onStackCreated, 
  hosts, 
  groups = [], 
  defaultHost,
  defaultScopeKind = 'host'
}: NewStackDialogProps) {
  const [scopeKind, setScopeKind] = useState<'host' | 'group'>(defaultScopeKind);
  const [scopeName, setScopeName] = useState('');
  const [stackName, setStackName] = useState('');
  const [creating, setCreating] = useState(false);
  const [nameError, setNameError] = useState('');

  // Extract unique groups from all hosts
  const allGroups = React.useMemo(() => {
    const groupSet = new Set<string>();
    hosts.forEach(host => {
      if (host.groups) {
        host.groups.forEach(g => groupSet.add(g));
      }
    });
    groups.forEach(g => groupSet.add(g));
    return Array.from(groupSet).sort();
  }, [hosts, groups]);

  // Set default host when dialog opens
  useEffect(() => {
    if (open) {
      if (defaultHost && scopeKind === 'host') {
        setScopeName(defaultHost);
      } else if (!scopeName) {
        // Set first available option as default
        if (scopeKind === 'host' && hosts.length > 0) {
          setScopeName(hosts[0].name);
        } else if (scopeKind === 'group' && allGroups.length > 0) {
          setScopeName(allGroups[0]);
        }
      }
    } else {
      // Reset form when dialog closes
      setStackName('');
      setNameError('');
      setScopeName('');
    }
  }, [open, defaultHost, scopeKind, hosts, allGroups]);

  // Update scopeName when switching between host/group
  useEffect(() => {
    if (scopeKind === 'host' && hosts.length > 0) {
      setScopeName(defaultHost || hosts[0].name);
    } else if (scopeKind === 'group' && allGroups.length > 0) {
      setScopeName(allGroups[0]);
    } else {
      setScopeName('');
    }
  }, [scopeKind, hosts, allGroups, defaultHost]);

  function dockerSanitizePreview(s: string): string {
    const lowered = s.toLowerCase().replaceAll(" ", "_");
    const stripped = lowered.replace(/[^a-z0-9_-]/g, "_");
    return stripped.replace(/^[-_]+|[-_]+$/g, "") || "default";
  }

  function hasUnsupportedChars(s: string): boolean {
    return /[^A-Za-z0-9 _-]/.test(s);
  }

  const validateStackName = (name: string) => {
    if (!name.trim()) {
      setNameError('Stack name is required');
      return false;
    }
    
    if (hasUnsupportedChars(name)) {
      const preview = dockerSanitizePreview(name);
      setNameError(`Docker Compose will normalize this to: ${preview}`);
      // This is a warning, not an error - allow creation
      return true;
    }
    
    setNameError('');
    return true;
  };

  const handleSubmit = async () => {
    if (!stackName.trim() || !scopeName) {
      errorLog('Stack name and scope are required');
      return;
    }

    if (!validateStackName(stackName)) {
      return;
    }

    setCreating(true);
    try {
      const endpoint = `/api/iac/scopes/${encodeURIComponent(scopeName)}/stacks`;

      const response = await fetch(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({
          stack_name: stackName.trim(),
        }),
      });

      if (response.ok) {
        infoLog(`Stack "${stackName}" created successfully for ${scopeKind} "${scopeName}"`);
        onStackCreated?.(scopeKind, scopeName, stackName.trim());
        onClose();
        setStackName('');
        setNameError('');
      } else {
        const error = await response.text();
        if (error.includes('already exists')) {
          setNameError('A stack with that name already exists');
        } else {
          errorLog('Failed to create stack:', error);
        }
      }
    } catch (error) {
      errorLog('Failed to create stack:', error);
    } finally {
      setCreating(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onClose}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Create New Stack</DialogTitle>
          <DialogDescription>
            Create a new Docker Compose stack for a host or group
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          {/* Scope Type Selection */}
          <div className="space-y-2">
            <Label>Scope Type</Label>
            <div className="grid grid-cols-2 gap-2">
              <Button
                type="button"
                variant={scopeKind === 'host' ? 'default' : 'outline'}
                onClick={() => setScopeKind('host')}
                className={scopeKind === 'host' ? 'bg-[#310937] hover:bg-[#2a0830]' : ''}
              >
                <Server className="h-4 w-4 mr-2" />
                Host
              </Button>
              <Button
                type="button"
                variant={scopeKind === 'group' ? 'default' : 'outline'}
                onClick={() => setScopeKind('group')}
                className={scopeKind === 'group' ? 'bg-[#310937] hover:bg-[#2a0830]' : ''}
              >
                <Users className="h-4 w-4 mr-2" />
                Group
              </Button>
            </div>
          </div>

          {/* Scope Selection */}
          <div className="space-y-2">
            <Label htmlFor="scope">
              {scopeKind === 'host' ? 'Select Host' : 'Select Group'}
            </Label>
            <Select value={scopeName} onValueChange={setScopeName}>
              <SelectTrigger id="scope">
                <SelectValue placeholder={`Choose a ${scopeKind}...`} />
              </SelectTrigger>
              <SelectContent>
                {scopeKind === 'host' ? (
                  hosts.map((host) => (
                    <SelectItem key={host.name} value={host.name}>
                      {host.name}
                      {host.alt_name && (
                        <span className="text-slate-500 ml-2">({host.alt_name})</span>
                      )}
                    </SelectItem>
                  ))
                ) : (
                  allGroups.map((group) => (
                    <SelectItem key={group} value={group}>
                      {group}
                    </SelectItem>
                  ))
                )}
              </SelectContent>
            </Select>
          </div>

          {/* Stack Name */}
          <div className="space-y-2">
            <Label htmlFor="stackname">Stack Name</Label>
            <Input
              id="stackname"
              value={stackName}
              onChange={(e) => {
                setStackName(e.target.value);
                validateStackName(e.target.value);
              }}
              placeholder="e.g., web-services, monitoring, databases"
              className={nameError && !nameError.includes('normalize') ? 'border-red-500' : ''}
            />
            {nameError && (
              <p className={`text-sm ${nameError.includes('normalize') ? 'text-yellow-400' : 'text-red-400'}`}>
                {nameError}
              </p>
            )}
          </div>

          {/* Preview */}
          {stackName && scopeName && (
            <div className="p-3 bg-slate-900/60 border border-slate-800 rounded-lg">
              <p className="text-xs text-slate-400 mb-1">Stack will be created at:</p>
              <p className="font-mono text-sm text-slate-200">
                {scopeKind === 'host' ? 'hosts' : 'groups'}/{scopeName}/{stackName}
              </p>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={creating}>
            Cancel
          </Button>
          <Button 
            onClick={handleSubmit} 
            disabled={creating || !stackName.trim() || !scopeName}
            className="bg-[#310937] hover:bg-[#2a0830]"
          >
            {creating ? (
              <>
                <Boxes className="h-4 w-4 mr-2 animate-pulse" />
                Creating...
              </>
            ) : (
              <>
                <Boxes className="h-4 w-4 mr-2" />
                Create Stack
              </>
            )}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}