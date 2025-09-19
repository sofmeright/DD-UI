// ui/src/components/EnhancedHostPicker.tsx
import { Host } from "@/types";
import { Card } from "@/components/ui/card";
import { Server, Users, Globe } from "lucide-react";

export type PickerOption = {
  type: 'all' | 'host' | 'group';
  value: string;
  label: string;
  address?: string;
};

interface EnhancedHostPickerProps {
  hosts: Host[];
  groups: string[];
  activeSelection: string;
  onSelectionChange: (selection: PickerOption) => void;
}

export default function EnhancedHostPicker({
  hosts, 
  groups, 
  activeSelection, 
  onSelectionChange
}: EnhancedHostPickerProps) {
  
  // Build options list
  const options: PickerOption[] = [
    { type: 'all', value: 'all', label: 'All Stacks' },
    ...groups.map(g => ({ 
      type: 'group' as const, 
      value: `group:${g}`, 
      label: g 
    })),
    ...hosts.map(h => ({ 
      type: 'host' as const, 
      value: `host:${h.name}`, 
      label: h.name,
      address: h.address || h.addr
    }))
  ];

  const currentOption = options.find(o => o.value === activeSelection) || options[0];
  
  const handleChange = (value: string) => {
    const option = options.find(o => o.value === value);
    if (option) {
      onSelectionChange(option);
    }
  };

  const getIcon = (type: string) => {
    switch (type) {
      case 'all': return <Globe className="h-4 w-4 text-slate-400" />;
      case 'group': return <Users className="h-4 w-4 text-blue-400" />;
      case 'host': return <Server className="h-4 w-4 text-green-400" />;
      default: return null;
    }
  };

  return (
    <Card className="bg-slate-800/60 border-slate-700 px-3 py-1.5">
      <div className="flex items-center gap-3">
        <div className="flex items-center gap-2">
          {getIcon(currentOption.type)}
          <select
            className="bg-slate-900/70 border border-slate-600 text-slate-200 text-sm rounded px-2 py-1 min-w-[200px] focus:outline-none focus:ring-2 focus:ring-slate-500 focus:border-slate-500"
            value={activeSelection}
            onChange={(e) => handleChange(e.target.value)}
          >
            <option value="all" className="bg-slate-900 text-slate-200">
              📊 All Stacks
            </option>
            
            {groups.length > 0 && (
              <optgroup label="Groups" className="bg-slate-800">
                {groups.map(g => (
                  <option key={`group:${g}`} value={`group:${g}`} className="bg-slate-900 text-slate-200">
                    👥 {g}
                  </option>
                ))}
              </optgroup>
            )}
            
            {hosts.length > 0 && (
              <optgroup label="Hosts" className="bg-slate-800">
                {hosts.map(h => (
                  <option key={`host:${h.name}`} value={`host:${h.name}`} className="bg-slate-900 text-slate-200">
                    🖥️ {h.name}
                  </option>
                ))}
              </optgroup>
            )}
          </select>
        </div>
        
        {currentOption.type === 'host' && currentOption.address && (
          <div className="text-slate-300 text-sm font-mono">{currentOption.address}</div>
        )}
        
        {currentOption.type === 'group' && (
          <div className="text-blue-400 text-sm">Group Configuration</div>
        )}
        
        {currentOption.type === 'all' && (
          <div className="text-slate-400 text-sm">All Host & Group Stacks</div>
        )}
      </div>
    </Card>
  );
}