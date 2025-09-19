import React, { useState } from 'react';
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
import { Textarea } from "@/components/ui/textarea";
import { Badge } from "@/components/ui/badge";
import { X, Plus, Trash2 } from "lucide-react";
import { infoLog, errorLog } from "@/utils/logging";
import ConfirmDialog from "@/components/ConfirmDialog";

interface AddHostDialogProps {
  open: boolean;
  onClose: () => void;
  onHostAdded?: () => void;
  hostToEdit?: any;  // Host data when editing
  isEditMode?: boolean;
}

export default function AddHostDialog({ open, onClose, onHostAdded, hostToEdit, isEditMode = false }: AddHostDialogProps) {
  const [hostData, setHostData] = useState({
    name: '',
    ansible_host: '',
    description: '',
    alt_name: '',
    tenant: '',
    owner: 'unassigned',
    tags: [] as string[],
    allowed_users: [] as string[],
    env: {} as Record<string, string>,
  });

  const [newTag, setNewTag] = useState('');
  const [newUser, setNewUser] = useState('');
  const [newEnvKey, setNewEnvKey] = useState('');
  const [newEnvValue, setNewEnvValue] = useState('');
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [tempClosedForDelete, setTempClosedForDelete] = useState(false);

  // Populate form when editing
  React.useEffect(() => {
    if (isEditMode && hostToEdit && open) {
      setHostData({
        name: hostToEdit.name || '',
        ansible_host: hostToEdit.addr || hostToEdit.address || '',
        description: hostToEdit.description || '',
        alt_name: hostToEdit.alt_name || '',
        tenant: hostToEdit.tenant || '',
        owner: hostToEdit.owner || 'unassigned',
        tags: hostToEdit.tags || [],
        allowed_users: hostToEdit.allowed_users || [],
        env: hostToEdit.env || {},
      });
    } else if (!open) {
      // Reset form when dialog closes
      resetForm();
    }
  }, [isEditMode, hostToEdit, open]);

  const handleSubmit = async () => {
    if (!hostData.name || !hostData.ansible_host) {
      errorLog('Host name and ansible_host are required');
      return;
    }

    setSaving(true);
    try {
      const url = isEditMode ? `/api/hosts/${encodeURIComponent(hostToEdit.name)}` : '/api/hosts';
      const method = isEditMode ? 'PUT' : 'POST';
      
      const response = await fetch(url, {
        method: method,
        headers: { 'Content-Type': 'application/json' },
        credentials: 'include',
        body: JSON.stringify({
          name: hostData.name,
          addr: hostData.ansible_host,
          description: hostData.description,
          alt_name: hostData.alt_name,
          tenant: hostData.tenant,
          owner: hostData.owner,
          tags: hostData.tags,
          allowed_users: hostData.allowed_users,
          env: hostData.env,
        }),
      });

      if (response.ok) {
        infoLog(isEditMode ? 'Host updated successfully' : 'Host added successfully');
        onHostAdded?.();
        onClose();
        resetForm();
      } else {
        const error = await response.text();
        errorLog(isEditMode ? 'Failed to update host:' : 'Failed to add host:', error);
      }
    } catch (error) {
      errorLog(isEditMode ? 'Failed to update host:' : 'Failed to add host:', error);
    } finally {
      setSaving(false);
    }
  };

  const resetForm = () => {
    setHostData({
      name: '',
      ansible_host: '',
      description: '',
      alt_name: '',
      tenant: '',
      owner: 'unassigned',
      tags: [],
      allowed_users: [],
      env: {},
    });
    setNewTag('');
    setNewUser('');
    setNewEnvKey('');
    setNewEnvValue('');
  };

  const addTag = () => {
    if (newTag && !hostData.tags.includes(newTag)) {
      setHostData({ ...hostData, tags: [...hostData.tags, newTag] });
      setNewTag('');
    }
  };

  const removeTag = (tag: string) => {
    setHostData({ ...hostData, tags: hostData.tags.filter(t => t !== tag) });
  };

  const addUser = () => {
    if (newUser && !hostData.allowed_users.includes(newUser)) {
      setHostData({ ...hostData, allowed_users: [...hostData.allowed_users, newUser] });
      setNewUser('');
    }
  };

  const removeUser = (user: string) => {
    setHostData({ ...hostData, allowed_users: hostData.allowed_users.filter(u => u !== user) });
  };

  const addEnvVar = () => {
    if (newEnvKey && !hostData.env[newEnvKey]) {
      setHostData({ ...hostData, env: { ...hostData.env, [newEnvKey]: newEnvValue } });
      setNewEnvKey('');
      setNewEnvValue('');
    }
  };

  const removeEnvVar = (key: string) => {
    const newEnv = { ...hostData.env };
    delete newEnv[key];
    setHostData({ ...hostData, env: newEnv });
  };

  const handleDelete = async () => {
    setShowDeleteConfirm(false);
    setDeleting(true);
    try {
      const response = await fetch(`/api/hosts/${encodeURIComponent(hostToEdit.name)}`, {
        method: 'DELETE',
        credentials: 'include',
      });

      if (response.ok) {
        infoLog('Host deleted successfully');
        onHostAdded?.(); // Trigger refresh
        onClose();
        resetForm();
      } else {
        const error = await response.text();
        errorLog('Failed to delete host:', error);
      }
    } catch (error) {
      errorLog('Failed to delete host:', error);
    } finally {
      setDeleting(false);
    }
  };

  return (
    <>
      <Dialog open={open && !tempClosedForDelete} onOpenChange={onClose}>
        <DialogContent className="max-w-2xl max-h-[80vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{isEditMode ? 'Edit Host' : 'Add Host'}</DialogTitle>
          <DialogDescription>
            {isEditMode ? 'Update host configuration in the inventory' : 'Add a new host to the inventory with all its configuration'}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          {/* Basic Information */}
          <div className="grid grid-cols-2 gap-4">
            <div>
              <Label htmlFor="hostname">Host Name *</Label>
              <Input
                id="hostname"
                value={hostData.name}
                onChange={(e) => setHostData({ ...hostData, name: e.target.value })}
                placeholder="e.g., web-server-01"
                disabled={isEditMode}  // Can't change host name when editing
              />
            </div>
            <div>
              <Label htmlFor="ansible_host">Ansible Host (IP/FQDN) *</Label>
              <Input
                id="ansible_host"
                value={hostData.ansible_host}
                onChange={(e) => setHostData({ ...hostData, ansible_host: e.target.value })}
                placeholder="e.g., 192.168.1.100"
              />
            </div>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <div>
              <Label htmlFor="alt_name">Alternative Name</Label>
              <Input
                id="alt_name"
                value={hostData.alt_name}
                onChange={(e) => setHostData({ ...hostData, alt_name: e.target.value })}
                placeholder="e.g., Production Web Server"
              />
            </div>
            <div>
              <Label htmlFor="owner">Owner</Label>
              <Input
                id="owner"
                value={hostData.owner}
                onChange={(e) => setHostData({ ...hostData, owner: e.target.value })}
                placeholder="e.g., devops-team"
              />
            </div>
          </div>

          <div>
            <Label htmlFor="tenant">Tenant</Label>
            <Input
              id="tenant"
              value={hostData.tenant}
              onChange={(e) => setHostData({ ...hostData, tenant: e.target.value })}
              placeholder="e.g., customer-a"
            />
          </div>

          <div>
            <Label htmlFor="description">Description</Label>
            <Textarea
              id="description"
              value={hostData.description}
              onChange={(e) => setHostData({ ...hostData, description: e.target.value })}
              placeholder="Describe the purpose of this host..."
              rows={3}
            />
          </div>

          {/* Tags */}
          <div>
            <Label>Tags</Label>
            <div className="flex gap-2 mb-2">
              <Input
                value={newTag}
                onChange={(e) => setNewTag(e.target.value)}
                placeholder="Add a tag"
                onKeyPress={(e) => e.key === 'Enter' && (e.preventDefault(), addTag())}
              />
              <Button onClick={addTag} size="sm" variant="outline">
                <Plus className="h-4 w-4" />
              </Button>
            </div>
            <div className="flex flex-wrap gap-2">
              {hostData.tags.map((tag) => (
                <Badge key={tag} variant="secondary" className="gap-1">
                  {tag}
                  <X
                    className="h-3 w-3 cursor-pointer"
                    onClick={() => removeTag(tag)}
                  />
                </Badge>
              ))}
            </div>
          </div>

          {/* Allowed Users */}
          <div>
            <Label>Allowed Users</Label>
            <div className="flex gap-2 mb-2">
              <Input
                value={newUser}
                onChange={(e) => setNewUser(e.target.value)}
                placeholder="Add a user"
                onKeyPress={(e) => e.key === 'Enter' && (e.preventDefault(), addUser())}
              />
              <Button onClick={addUser} size="sm" variant="outline">
                <Plus className="h-4 w-4" />
              </Button>
            </div>
            <div className="flex flex-wrap gap-2">
              {hostData.allowed_users.map((user) => (
                <Badge key={user} variant="secondary" className="gap-1">
                  {user}
                  <X
                    className="h-3 w-3 cursor-pointer"
                    onClick={() => removeUser(user)}
                  />
                </Badge>
              ))}
            </div>
          </div>

          {/* Environment Variables */}
          <div>
            <Label>Environment Variables</Label>
            <div className="flex gap-2 mb-2">
              <Input
                value={newEnvKey}
                onChange={(e) => setNewEnvKey(e.target.value)}
                placeholder="Key"
              />
              <Input
                value={newEnvValue}
                onChange={(e) => setNewEnvValue(e.target.value)}
                placeholder="Value"
              />
              <Button onClick={addEnvVar} size="sm" variant="outline">
                <Plus className="h-4 w-4" />
              </Button>
            </div>
            <div className="space-y-2">
              {Object.entries(hostData.env).map(([key, value]) => (
                <div key={key} className="flex items-center gap-2 p-2 bg-slate-900 rounded">
                  <span className="font-mono text-sm text-blue-400">{key}</span>
                  <span className="text-slate-500">=</span>
                  <span className="font-mono text-sm text-green-400 flex-1">{value}</span>
                  <X
                    className="h-4 w-4 cursor-pointer text-slate-400 hover:text-red-400"
                    onClick={() => removeEnvVar(key)}
                  />
                </div>
              ))}
            </div>
          </div>
        </div>

        <DialogFooter className="flex justify-between">
          <div className="flex-1">
            {isEditMode && (
              <Button 
                variant="outline" 
                onClick={() => {
                  setTempClosedForDelete(true);
                  setShowDeleteConfirm(true);
                }}
                disabled={deleting || saving}
                className="border-red-800 text-red-400 hover:bg-red-900/20"
              >
                {deleting ? (
                  <>Deleting...</>
                ) : (
                  <>
                    <Trash2 className="h-4 w-4 mr-1" />
                    Delete Host
                  </>
                )}
              </Button>
            )}
          </div>
          <div className="flex gap-2">
            <Button variant="outline" onClick={onClose} disabled={saving || deleting}>
              Cancel
            </Button>
            <Button onClick={handleSubmit} disabled={saving || deleting || !hostData.name || !hostData.ansible_host}>
              {saving ? (isEditMode ? 'Updating...' : 'Adding...') : (isEditMode ? 'Update Host' : 'Add Host')}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>

    <ConfirmDialog
      isOpen={showDeleteConfirm}
      title="Delete Host"
      message={`Are you sure you want to delete host "${hostToEdit?.name || hostData.name}"?\n\nThis action cannot be undone and will remove the host from your inventory.`}
      variant="danger"
      confirmText="Delete"
      cancelText="Cancel"
      onConfirm={() => {
        setTempClosedForDelete(false);
        handleDelete();
      }}
      onCancel={() => {
        setShowDeleteConfirm(false);
        setTempClosedForDelete(false);
      }}
    />
    </>
  );
}