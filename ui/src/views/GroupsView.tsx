import React, { useState, useEffect } from 'react';
import { ChevronRight, ChevronDown, FolderOpen, Folder, Plus, Trash2, Save, Users, Server, Layers, Edit2, X } from 'lucide-react';
import { Host } from "@/types";
import { handle401 } from "@/utils/auth";
import { debugLog, errorLog, infoLog } from '@/utils/logging';
import GitSyncToggle from "@/components/GitSyncToggle";
import DevOpsToggle from '@/components/DevOpsToggle';
import { Button } from "@/components/ui/button";

interface Group {
  name: string;
  description?: string;
  tags?: string[];
  alt_name?: string;
  tenant?: string;
  allowed_users?: string[];
  owner?: string;
  env?: Record<string, string>;
  vars?: Record<string, any>;
  hosts?: string[];
  children?: string[];
  host_count?: number;
  stack_count?: number;
}

interface GroupMembership {
  group_name: string;
  direct_member: boolean;
  inherited_from?: string;
}

export default function GroupsView({ hosts }: { hosts: Host[] }) {
  const [groups, setGroups] = useState<Group[]>([]);
  const [selectedGroup, setSelectedGroup] = useState<Group | null>(null);
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(false);
  const [editingGroup, setEditingGroup] = useState<Group | null>(null);
  const [groupHosts, setGroupHosts] = useState<Host[]>([]);
  const [savingGroup, setSavingGroup] = useState(false);
  const [showNewGroupForm, setShowNewGroupForm] = useState(false);
  const [newGroup, setNewGroup] = useState<Partial<Group & { parent?: string }>>({
    name: '',
    description: '',
    tags: [],
    owner: 'unassigned',
    parent: ''
  });
  const [showHostPicker, setShowHostPicker] = useState(false);
  const [availableHosts, setAvailableHosts] = useState<any[]>([]);
  const [selectedHostNames, setSelectedHostNames] = useState<string[]>([]);
  const [treeWidth, setTreeWidth] = useState(320); // Initial width of tree pane
  const [isResizing, setIsResizing] = useState(false);

  useEffect(() => {
    fetchGroups();
    fetchAvailableHosts();
  }, []);

  useEffect(() => {
    if (selectedGroup) {
      fetchGroupHosts(selectedGroup.name);
    }
  }, [selectedGroup]);

  // Handle resizable divider
  useEffect(() => {
    const handleMouseMove = (e: MouseEvent) => {
      if (!isResizing) return;
      
      const newWidth = Math.max(200, Math.min(600, e.clientX));
      setTreeWidth(newWidth);
    };

    const handleMouseUp = () => {
      setIsResizing(false);
    };

    if (isResizing) {
      document.addEventListener('mousemove', handleMouseMove);
      document.addEventListener('mouseup', handleMouseUp);
    }

    return () => {
      document.removeEventListener('mousemove', handleMouseMove);
      document.removeEventListener('mouseup', handleMouseUp);
    };
  }, [isResizing]);

  const fetchGroups = async () => {
    setLoading(true);
    try {
      const response = await fetch('/api/groups', {
        credentials: 'include',
      });
      if (response.status === 401) {
        handle401();
        return;
      }
      if (response.ok) {
        const data = await response.json();
        setGroups(data || []);
        debugLog('Fetched groups:', data);
      } else {
        const errorText = await response.text();
        errorLog('Failed to fetch groups - Server returned:', errorText);
      }
    } catch (error) {
      errorLog('Failed to fetch groups - Exception:', error);
    } finally {
      setLoading(false);
    }
  };

  const fetchAvailableHosts = async () => {
    try {
      const response = await fetch('/api/hosts', {
        credentials: 'include',
      });
      if (response.ok) {
        const data = await response.json();
        setAvailableHosts(data.items || []);
      }
    } catch (error) {
      errorLog('Failed to fetch available hosts:', error);
    }
  };

  const fetchGroupHosts = async (groupName: string) => {
    try {
      const response = await fetch(`/api/groups/${groupName}/hosts`, {
        credentials: 'include',
      });
      if (response.ok) {
        const data = await response.json();
        setGroupHosts(data || []);
        debugLog('Fetched hosts for group', groupName, ':', data);
      }
    } catch (error) {
      errorLog('Failed to fetch group hosts:', error);
    }
  };

  const toggleGroupExpanded = (groupName: string) => {
    setExpandedGroups(prev => {
      const next = new Set(prev);
      if (next.has(groupName)) {
        next.delete(groupName);
      } else {
        next.add(groupName);
      }
      return next;
    });
  };

  const handleSelectGroup = async (group: Group) => {
    setSelectedGroup(group);
    setEditingGroup(null);
    
    // Fetch full group details
    try {
      const response = await fetch(`/api/groups/${group.name}`, {
        credentials: 'include',
      });
      if (response.ok) {
        const data = await response.json();
        setSelectedGroup(data);
      }
    } catch (error) {
      errorLog('Failed to fetch group details:', error);
    }
  };

  const handleEditGroup = () => {
    if (selectedGroup) {
      setEditingGroup({ ...selectedGroup });
    }
  };

  const handleSaveGroup = async () => {
    if (!editingGroup) return;
    
    setSavingGroup(true);
    try {
      const response = await fetch(`/api/groups/${editingGroup.name}`, {
        method: 'PUT',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(editingGroup),
      });
      
      if (response.ok) {
        infoLog('Group updated successfully');
        setSelectedGroup(editingGroup);
        setEditingGroup(null);
        fetchGroups();
      }
    } catch (error) {
      errorLog('Failed to update group:', error);
    } finally {
      setSavingGroup(false);
    }
  };

  const handleCreateGroup = async () => {
    console.log('CREATE BUTTON CLICKED!'); // Temporary debug line
    debugLog('handleCreateGroup called with newGroup:', newGroup);
    
    if (!newGroup.name) {
      errorLog('Cannot create group: Name is required');
      return;
    }
    
    try {
      // Prepare payload ensuring parent field is included correctly
      const payload = {
        name: newGroup.name,
        description: newGroup.description || '',
        tags: newGroup.tags || [],
        parent: newGroup.parent || '',  // Make sure this is 'parent' not 'parent_id'
        owner: newGroup.owner || 'unassigned',
        alt_name: newGroup.alt_name || '',
        tenant: newGroup.tenant || '',
        allowed_users: newGroup.allowed_users || [],
        env: newGroup.env || {}
      };
      
      infoLog('Creating group with payload:', JSON.stringify(payload));
      
      const response = await fetch('/api/groups', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      
      debugLog('Create group response status:', response.status);
      
      if (response.ok) {
        const result = await response.json();
        infoLog('Group created successfully:', JSON.stringify(result));
        setShowNewGroupForm(false);
        setNewGroup({
          name: '',
          description: '',
          tags: [],
          owner: 'unassigned',
          parent: ''
        });
        await fetchGroups();
      } else {
        const errorText = await response.text();
        errorLog(`Failed to create group - Server returned ${response.status}: ${errorText}`);
      }
    } catch (error) {
      errorLog('Failed to create group - Exception:', JSON.stringify(error));
    }
  };

  const handleDeleteGroup = async (groupName: string) => {
    if (!confirm('Are you sure you want to delete this group?')) return;
    
    try {
      const response = await fetch(`/api/groups/${groupName}`, {
        method: 'DELETE',
        credentials: 'include',
      });
      
      if (response.ok) {
        infoLog('Group deleted successfully');
        if (selectedGroup?.name === groupName) {
          setSelectedGroup(null);
        }
        fetchGroups();
      }
    } catch (error) {
      errorLog('Failed to delete group:', error);
    }
  };

  const handleAddHostsToGroup = async () => {
    if (!selectedGroup || selectedHostNames.length === 0) return;
    
    try {
      const response = await fetch(`/api/groups/${selectedGroup.name}/hosts`, {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ hosts: selectedHostNames }),
      });
      
      if (response.ok) {
        infoLog('Hosts added to group successfully');
        fetchGroupHosts(selectedGroup.name);
        setShowHostPicker(false);
        setSelectedHostNames([]);
      } else {
        const error = await response.text();
        errorLog('Failed to add hosts to group:', error);
      }
    } catch (error) {
      errorLog('Failed to add hosts to group:', error);
    }
  };

  const handleRemoveHostFromGroup = async (hostName: string) => {
    if (!selectedGroup) return;
    
    try {
      const response = await fetch(`/api/groups/${selectedGroup.name}/hosts/${hostName}`, {
        method: 'DELETE',
        credentials: 'include',
      });
      
      if (response.ok) {
        infoLog('Host removed from group successfully');
        fetchGroupHosts(selectedGroup.name);
      }
    } catch (error) {
      errorLog('Failed to remove host from group:', error);
    }
  };

  const renderGroupTree = (groupList: Group[], level = 0, parentGroup?: Group) => {
    // If this is the root level, only show groups that aren't children of any other group
    if (level === 0 && !parentGroup) {
      const childGroupNames = new Set<string>();
      groups.forEach(g => {
        if (g.children) {
          g.children.forEach(child => childGroupNames.add(child));
        }
      });
      groupList = groupList.filter(g => !childGroupNames.has(g.name));
    }
    
    return groupList.map(group => (
      <div key={group.name} style={{ marginLeft: `${level * 20}px` }}>
        <div
          className={`flex items-center gap-2 p-2 rounded cursor-pointer hover:bg-slate-800/50 ${
            selectedGroup?.name === group.name ? 'bg-slate-800' : ''
          }`}
          onClick={() => {
            handleSelectGroup(group);
            // Also toggle expansion when clicking the folder
            if (group.children?.length > 0 || group.host_count > 0) {
              toggleGroupExpanded(group.name);
            }
          }}
        >
          <button
            onClick={(e) => {
              e.stopPropagation();
              toggleGroupExpanded(group.name);
              // Also select the group when using chevron
              handleSelectGroup(group);
            }}
            className="p-0.5"
          >
            {(group.children && group.children.length > 0) || (group.hosts && group.hosts.length > 0) || group.host_count > 0 ? (
              expandedGroups.has(group.name) ? <ChevronDown className="w-4 h-4" /> : <ChevronRight className="w-4 h-4" />
            ) : (
              <span className="w-4" />
            )}
          </button>
          {expandedGroups.has(group.name) ? 
            <FolderOpen className="w-4 h-4 text-yellow-500" /> : 
            <Folder className="w-4 h-4 text-yellow-600" />
          }
          <span className="text-sm text-slate-200">{group.name}</span>
          <span className="text-xs text-slate-500">
            ({group.host_count} hosts{group.stack_count > 0 ? `, ${group.stack_count} stacks` : ''})
          </span>
        </div>
        
        {expandedGroups.has(group.name) && (
          <>
            {/* Render hosts in this group - show when expanded */}
            {group.hosts && group.hosts.length > 0 && (
              <div style={{ marginLeft: `${(level + 1) * 20}px` }}>
                {group.hosts.map((hostname: string) => (
                  <div key={`host-${hostname}`} className="flex items-center gap-2 p-1 pl-6 hover:bg-slate-800/30">
                    <Server className="w-4 h-4 text-blue-500" />
                    <span className="text-sm text-slate-300">{hostname}</span>
                  </div>
                ))}
              </div>
            )}
            
            {/* Render child groups */}
            {group.children && Array.isArray(group.children) && group.children.length > 0 && 
              renderGroupTree(groups.filter(g => group.children?.includes(g.name)), level + 1, group)
            }
          </>
        )}
      </div>
    ));
  };

  const renderVariableEditor = () => {
    const group = editingGroup || selectedGroup;
    if (!group) return null;

    return (
      <div className="space-y-2">
        <div className="text-sm font-medium text-slate-300">Variables</div>
        <div className="space-y-1">
          {Object.entries(group.vars || {}).map(([key, value]) => (
            <div key={key} className="flex gap-2">
              <input
                className="flex-1 px-2 py-1 bg-slate-900/60 border border-slate-700 rounded text-sm text-white"
                value={key}
                disabled={!editingGroup}
                onChange={(e) => {
                  if (editingGroup) {
                    const newVars = { ...editingGroup.vars };
                    delete newVars[key];
                    newVars[e.target.value] = value;
                    setEditingGroup({ ...editingGroup, vars: newVars });
                  }
                }}
              />
              <input
                className="flex-1 px-2 py-1 bg-slate-900/60 border border-slate-700 rounded text-sm text-white"
                value={value}
                disabled={!editingGroup}
                onChange={(e) => {
                  if (editingGroup) {
                    setEditingGroup({
                      ...editingGroup,
                      vars: { ...editingGroup.vars, [key]: e.target.value }
                    });
                  }
                }}
              />
              {editingGroup && (
                <button
                  onClick={() => {
                    const newVars = { ...editingGroup.vars };
                    delete newVars[key];
                    setEditingGroup({ ...editingGroup, vars: newVars });
                  }}
                  className="p-1 text-red-400 hover:text-red-300"
                >
                  <Trash2 className="w-4 h-4" />
                </button>
              )}
            </div>
          ))}
          {editingGroup && (
            <button
              onClick={() => {
                const newKey = `var_${Object.keys(editingGroup.vars || {}).length + 1}`;
                setEditingGroup({
                  ...editingGroup,
                  vars: { ...editingGroup.vars, [newKey]: '' }
                });
              }}
              className="text-xs text-blue-400 hover:text-blue-300"
            >
              + Add Variable
            </button>
          )}
        </div>
      </div>
    );
  };

  return (
    <div className="h-full flex select-none">
      {/* Left Panel - Tree View */}
      <div 
        className="border-r border-slate-800 bg-slate-950 overflow-y-auto flex-shrink-0"
        style={{ width: `${treeWidth}px` }}
      >
        <div className="p-4 border-b border-slate-800">
          <div className="flex items-center justify-between mb-2">
            <div className="text-lg font-semibold text-white">Groups</div>
            <button
              onClick={() => setShowNewGroupForm(true)}
              className="p-1 hover:bg-slate-800 rounded"
              title="Create new group"
            >
              <Plus className="w-5 h-5" />
            </button>
          </div>
          <div className="text-xs text-slate-500">
            Manage host groups and their configurations
          </div>
        </div>
        
        {/* New Group Form */}
        {showNewGroupForm && (
          <div className="p-4 border-b border-slate-800 bg-slate-900/50">
            <div className="space-y-2">
              <input
                placeholder="Group name"
                className="w-full px-2 py-1 bg-slate-900/60 border border-slate-700 rounded text-sm text-white"
                value={newGroup.name}
                onChange={(e) => setNewGroup({ ...newGroup, name: e.target.value })}
              />
              <input
                placeholder="Description (optional)"
                className="w-full px-2 py-1 bg-slate-900/60 border border-slate-700 rounded text-sm text-white"
                value={newGroup.description}
                onChange={(e) => setNewGroup({ ...newGroup, description: e.target.value })}
              />
              <select
                className="w-full px-2 py-1 bg-slate-900/60 border border-slate-700 rounded text-sm text-white"
                value={newGroup.parent || ''}
                onChange={(e) => setNewGroup({ ...newGroup, parent: e.target.value })}
              >
                <option value="">No parent (top-level group)</option>
                {groups.map(g => (
                  <option key={g.name} value={g.name}>{g.name}</option>
                ))}
              </select>
              <div className="flex gap-2">
                <Button
                  size="sm"
                  onClick={handleCreateGroup}
                  disabled={!newGroup.name}
                >
                  Create
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setShowNewGroupForm(false);
                    setNewGroup({
                      name: '',
                      description: '',
                      tags: [],
                      owner: 'unassigned',
                      parent: ''
                    });
                  }}
                >
                  Cancel
                </Button>
              </div>
            </div>
          </div>
        )}
        
        {/* Groups Tree */}
        <div className="p-2">
          {loading ? (
            <div className="text-sm text-slate-500 p-4">Loading groups...</div>
          ) : groups.length === 0 ? (
            <div className="text-sm text-slate-500 p-4">No groups found</div>
          ) : (
            renderGroupTree(groups)
          )}
        </div>
      </div>

      {/* Resizable Divider */}
      <div
        className={`w-1 bg-slate-800 hover:bg-slate-700 cursor-col-resize transition-colors ${
          isResizing ? 'bg-slate-600' : ''
        }`}
        onMouseDown={(e) => {
          e.preventDefault();
          setIsResizing(true);
        }}
      />

      {/* Right Panel - Group Details */}
      <div className="flex-1 bg-slate-925 flex flex-col">
        {/* Top toolbar with GitSync */}
        <div className="flex items-center justify-end p-3 border-b border-slate-800">
          <GitSyncToggle />
        </div>
        
        <div className="flex-1 overflow-y-auto">
        {selectedGroup ? (
          <div className="p-6 space-y-6">
            {/* Header */}
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-4">
                <FolderOpen className="w-8 h-8 text-yellow-500" />
                <div>
                  <h2 className="text-2xl font-semibold text-white">{selectedGroup.name}</h2>
                  <div className="flex items-center gap-4 mt-1">
                    <div className="flex items-center gap-1 text-sm text-slate-400">
                      <Users className="w-4 h-4" />
                      <span>{selectedGroup.host_count} hosts</span>
                    </div>
                    <div className="flex items-center gap-1 text-sm text-slate-400">
                      <Layers className="w-4 h-4" />
                      <span>{selectedGroup.stack_count} stacks</span>
                    </div>
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-2">
                {editingGroup ? (
                  <>
                    <Button
                      onClick={handleSaveGroup}
                      disabled={savingGroup}
                      className="bg-green-600 hover:bg-green-700"
                    >
                      <Save className="w-4 h-4 mr-1" />
                      Save
                    </Button>
                    <Button
                      onClick={() => setEditingGroup(null)}
                      variant="ghost"
                    >
                      <X className="w-4 h-4 mr-1" />
                      Cancel
                    </Button>
                  </>
                ) : (
                  <>
                    <Button
                      onClick={handleEditGroup}
                      variant="outline"
                    >
                      <Edit2 className="w-4 h-4 mr-1" />
                      Edit
                    </Button>
                    <Button
                      onClick={() => handleDeleteGroup(selectedGroup.name)}
                      variant="outline"
                      className="border-red-800 text-red-400 hover:bg-red-900/20"
                    >
                      <Trash2 className="w-4 h-4 mr-1" />
                      Delete
                    </Button>
                  </>
                )}
                <DevOpsToggle level="host" groupName={selectedGroup.name} />
              </div>
            </div>

            {/* Group Information */}
            <div className="grid grid-cols-2 gap-6">
              <div className="space-y-4">
                <div>
                  <label className="text-sm font-medium text-slate-400">Description</label>
                  {editingGroup ? (
                    <textarea
                      className="w-full mt-1 px-3 py-2 bg-slate-900/60 border border-slate-700 rounded text-white"
                      rows={3}
                      value={editingGroup.description || ''}
                      onChange={(e) => setEditingGroup({ ...editingGroup, description: e.target.value })}
                    />
                  ) : (
                    <div className="mt-1 text-slate-200">
                      {selectedGroup.description || <span className="text-slate-500">No description</span>}
                    </div>
                  )}
                </div>

                <div>
                  <label className="text-sm font-medium text-slate-400">Tags</label>
                  {editingGroup ? (
                    <input
                      className="w-full mt-1 px-3 py-2 bg-slate-900/60 border border-slate-700 rounded text-white"
                      value={editingGroup.tags?.join(', ') || ''}
                      onChange={(e) => setEditingGroup({
                        ...editingGroup,
                        tags: e.target.value.split(',').map(t => t.trim()).filter(Boolean)
                      })}
                      placeholder="tag1, tag2, tag3"
                    />
                  ) : (
                    <div className="mt-1 flex flex-wrap gap-1">
                      {selectedGroup.tags?.length > 0 ? (
                        selectedGroup.tags.map(tag => (
                          <span key={tag} className="px-2 py-0.5 bg-slate-800 rounded text-xs text-slate-300">
                            {tag}
                          </span>
                        ))
                      ) : (
                        <span className="text-slate-500">No tags</span>
                      )}
                    </div>
                  )}
                </div>

                <div>
                  <label className="text-sm font-medium text-slate-400">Owner</label>
                  {editingGroup ? (
                    <input
                      className="w-full mt-1 px-3 py-2 bg-slate-900/60 border border-slate-700 rounded text-white"
                      value={editingGroup.owner || ''}
                      onChange={(e) => setEditingGroup({ ...editingGroup, owner: e.target.value })}
                    />
                  ) : (
                    <div className="mt-1 text-slate-200">{selectedGroup.owner || 'unassigned'}</div>
                  )}
                </div>
              </div>

              <div className="space-y-4">
                {renderVariableEditor()}
              </div>
            </div>

            {/* Member Hosts */}
            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <h3 className="text-lg font-medium text-white">Member Hosts</h3>
                <Button
                  size="sm"
                  onClick={() => {
                    setShowHostPicker(true);
                    setSelectedHostNames([]);
                  }}
                >
                  <Plus className="w-4 h-4 mr-1" />
                  Add Hosts
                </Button>
              </div>
              <div className="grid grid-cols-2 gap-4">
                {groupHosts.map((host: any) => (
                  <div key={host.name} className="flex items-center justify-between p-3 bg-slate-900/60 border border-slate-800 rounded">
                    <div className="flex items-center gap-3">
                      <Server className="w-5 h-5 text-slate-400" />
                      <div>
                        <div className="text-sm font-medium text-white">{host.name}</div>
                        <div className="text-xs text-slate-500">{host.addr || host.address}</div>
                      </div>
                    </div>
                    <button
                      onClick={() => handleRemoveHostFromGroup(host.name)}
                      className="p-1 text-red-400 hover:text-red-300"
                      title="Remove from group"
                    >
                      <X className="w-4 h-4" />
                    </button>
                  </div>
                ))}
                {groupHosts.length === 0 && (
                  <div className="col-span-2 text-center py-8 text-slate-500">
                    No hosts in this group
                  </div>
                )}
              </div>
            </div>

            {/* Stack Summary */}
            {selectedGroup.stack_count > 0 && (
              <div className="mt-6 p-4 bg-slate-900/40 border border-slate-800 rounded">
                <div className="text-sm text-slate-400">
                  This group has <span className="text-white font-medium">{selectedGroup.stack_count}</span> stacks deployed.
                  <a href="#" className="ml-2 text-blue-400 hover:text-blue-300">
                    View in Stacks →
                  </a>
                </div>
              </div>
            )}
          </div>
        ) : (
          <div className="h-full flex items-center justify-center">
            <div className="text-center space-y-4">
              <Folder className="w-16 h-16 text-slate-700 mx-auto" />
              <div className="text-xl font-semibold text-slate-400">Select a Group</div>
              <div className="text-sm text-slate-500 max-w-md">
                Choose a group from the tree on the left to view and edit its details
              </div>
            </div>
          </div>
        )}
        </div>
      </div>
      
      {/* Host Picker Modal */}
      {showHostPicker && (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
          <div className="bg-slate-900 border border-slate-700 rounded-lg p-6 w-[600px] max-h-[80vh] overflow-y-auto">
            <h3 className="text-xl font-semibold text-white mb-4">Add Hosts to Group</h3>
            
            <div className="space-y-2 mb-4">
              {availableHosts.map((host: any) => {
                const isInGroup = groupHosts.some((gh: any) => gh.name === host.name);
                return (
                  <label
                    key={host.name}
                    className={`flex items-center gap-3 p-3 rounded border ${
                      isInGroup 
                        ? 'bg-slate-800/50 border-slate-700 opacity-50 cursor-not-allowed' 
                        : 'bg-slate-900/60 border-slate-700 hover:bg-slate-800/50 cursor-pointer'
                    }`}
                  >
                    <input
                      type="checkbox"
                      disabled={isInGroup}
                      checked={selectedHostNames.includes(host.name)}
                      onChange={(e) => {
                        if (e.target.checked) {
                          setSelectedHostNames([...selectedHostNames, host.name]);
                        } else {
                          setSelectedHostNames(selectedHostNames.filter(name => name !== host.name));
                        }
                      }}
                      className="rounded border-slate-600"
                    />
                    <Server className="w-4 h-4 text-blue-500" />
                    <div className="flex-1">
                      <div className="text-sm font-medium text-white">{host.name}</div>
                      <div className="text-xs text-slate-400">{host.addr || host.address}</div>
                    </div>
                    {isInGroup && <span className="text-xs text-slate-500">Already in group</span>}
                  </label>
                );
              })}
            </div>
            
            <div className="flex justify-end gap-2">
              <Button
                variant="ghost"
                onClick={() => {
                  setShowHostPicker(false);
                  setSelectedHostNames([]);
                }}
              >
                Cancel
              </Button>
              <Button
                onClick={handleAddHostsToGroup}
                disabled={selectedHostNames.length === 0}
              >
                Add {selectedHostNames.length} Host{selectedHostNames.length !== 1 ? 's' : ''}
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}