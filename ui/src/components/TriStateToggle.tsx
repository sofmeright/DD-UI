import React from 'react';
import { Settings, Circle, CheckCircle, XCircle } from 'lucide-react';

interface TriStateToggleProps {
  value: boolean | null;  // null = unset, false = off, true = on
  onChange: (value: boolean | null) => void;
  label?: string;
  disabled?: boolean;
  inheritedFrom?: string;  // Shows where the effective value comes from
  scope?: string;  // Shows the scope (e.g., "global", "host", "stack")
  className?: string;
  compact?: boolean;  // For minimal space usage
}

export default function TriStateToggle({ 
  value, 
  onChange, 
  label = 'DevOps',
  disabled = false,
  inheritedFrom,
  scope,
  className = '',
  compact = false
}: TriStateToggleProps) {
  // Cycle through states: unset -> off -> on -> unset
  const handleClick = () => {
    if (disabled) return;
    
    if (value === null) {
      onChange(false);  // unset -> off
    } else if (value === false) {
      onChange(true);   // off -> on
    } else {
      onChange(null);   // on -> unset
    }
  };

  // Get display state
  const getStateDisplay = () => {
    if (value === null) return 'Unset';
    if (value === false) return 'Off';
    return 'On';
  };

  // Get icon based on state
  const getIcon = () => {
    const iconClass = `w-3 h-3 ${disabled ? 'animate-spin' : ''}`;
    if (value === null) return <Circle className={iconClass} />;
    if (value === false) return <XCircle className={iconClass} />;
    return <CheckCircle className={iconClass} />;
  };

  // Get color based on state
  const getColor = () => {
    if (disabled) return 'text-slate-500';
    if (value === null) return 'text-slate-400 hover:text-slate-300';  // Unset - gray
    if (value === false) return 'text-red-400 hover:text-red-300';     // Off - red
    return 'text-green-400 hover:text-green-300';                      // On - green
  };

  // Build tooltip
  const getTooltip = () => {
    const stateStr = getStateDisplay();
    const inheritStr = inheritedFrom ? ` (inherits from ${inheritedFrom})` : '';
    const nextState = value === null ? 'off' : value === false ? 'on' : 'unset';
    return `${label}: ${stateStr}${inheritStr} - Click to set ${nextState}`;
  };

  if (compact) {
    // Compact mode - just icon and state
    return (
      <button
        onClick={handleClick}
        disabled={disabled}
        className={`flex items-center gap-1 px-2 py-0.5 bg-slate-900/60 border border-slate-700 rounded-full transition-all hover:bg-slate-800/60 ${getColor()} text-xs ${className}`}
        title={getTooltip()}
      >
        {getIcon()}
        <span>{getStateDisplay()}</span>
      </button>
    );
  }

  // Normal mode with label
  return (
    <div className={`flex items-center gap-2 ${className}`}>
      <span className="text-xs text-slate-400">{label}:</span>
      <button
        onClick={handleClick}
        disabled={disabled}
        className={`flex items-center gap-1 px-2 py-0.5 bg-slate-900/60 border border-slate-700 rounded-full transition-all hover:bg-slate-800/60 ${getColor()} text-xs`}
        title={getTooltip()}
      >
        {getIcon()}
        <span>
          {getStateDisplay()}
          {scope && <span className="text-slate-500 ml-1">({scope})</span>}
        </span>
      </button>
    </div>
  );
}