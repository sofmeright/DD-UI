// ui/src/components/ui/switch.tsx
import React from "react";

export function Switch({
  checked,
  onCheckedChange,
  disabled = false,
  id,
}: {
  checked: boolean;
  onCheckedChange: (v: boolean) => void;
  disabled?: boolean;
  id?: string;
}) {
  const toggle = () => {
    if (!disabled) onCheckedChange(!checked);
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (disabled) return;
    if (e.key === " " || e.key === "Enter") {
      e.preventDefault();
      toggle();
    }
  };

  return (
    <button
      id={id}
      type="button"
      role="switch"
      aria-checked={checked}
      aria-disabled={disabled}
      disabled={disabled}
      tabIndex={disabled ? -1 : 0}
      onClick={toggle}
      onKeyDown={onKeyDown}
      className={`h-6 w-10 rounded-full border border-slate-700 transition ${
        disabled ? "opacity-50 cursor-not-allowed" : "cursor-pointer"
      } ${checked ? "bg-brand/70" : "bg-slate-800"}`}
    >
      <span
        className={`block h-5 w-5 rounded-full bg-white transition transform ${
          checked ? "translate-x-4" : "translate-x-0.5"
        } mt-0.5 ml-0.5`}
      />
    </button>
  );
}
