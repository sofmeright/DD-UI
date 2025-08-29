import * as React from "react";
import * as SwitchPr from "@radix-ui/react-switch";
import { cn } from "@/lib/utils";

export interface SwitchProps extends React.ComponentPropsWithoutRef<typeof SwitchPr.Root> {
  checked?: boolean;
  onCheckedChange?: (checked: boolean) => void;
}

export const Switch = React.forwardRef<
  React.ElementRef<typeof SwitchPr.Root>,
  SwitchProps
>(({ className, checked, onCheckedChange, ...props }, ref) => (
  <SwitchPr.Root
    ref={ref}
    className={cn(
      "peer inline-flex h-5 w-9 shrink-0 cursor-pointer items-center rounded-full border border-slate-700 bg-slate-900 transition-colors data-[state=checked]:bg-[#74ecbe]",
      className
    )}
    checked={checked}
    onCheckedChange={onCheckedChange}
    {...props}
  >
    <SwitchPr.Thumb
      className={cn(
        "pointer-events-none block h-4 w-4 translate-x-0.5 rounded-full bg-white shadow transition-transform data-[state=checked]:translate-x-[18px]"
      )}
    />
  </SwitchPr.Root>
));
Switch.displayName = "Switch";
