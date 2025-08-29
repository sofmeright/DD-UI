import * as React from "react";
import * as TabsPr from "@radix-ui/react-tabs";
import { cn } from "@/lib/utils";

export const Tabs = TabsPr.Root;

export const TabsList = React.forwardRef<
  React.ElementRef<typeof TabsPr.List>,
  React.ComponentPropsWithoutRef<typeof TabsPr.List>
>(({ className, ...props }, ref) => (
  <TabsPr.List
    ref={ref}
    className={cn("inline-flex items-center gap-1 rounded-lg p-1", className)}
    {...props}
  />
));
TabsList.displayName = "TabsList";

export const TabsTrigger = React.forwardRef<
  React.ElementRef<typeof TabsPr.Trigger>,
  React.ComponentPropsWithoutRef<typeof TabsPr.Trigger>
>(({ className, ...props }, ref) => (
  <TabsPr.Trigger
    ref={ref}
    className={cn(
      "inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium text-slate-300 data-[state=active]:bg-slate-800 data-[state=active]:text-white",
      className
    )}
    {...props}
  />
));
TabsTrigger.displayName = "TabsTrigger";

export const TabsContent = React.forwardRef<
  React.ElementRef<typeof TabsPr.Content>,
  React.ComponentPropsWithoutRef<typeof TabsPr.Content>
>(({ className, ...props }, ref) => (
  <TabsPr.Content ref={ref} className={cn("mt-2", className)} {...props} />
));
TabsContent.displayName = "TabsContent";
