import * as React from "react";
import { cn } from "@/lib/utils";

interface SelectProps {
  value?: string;
  onValueChange?: (value: string) => void;
  children: React.ReactNode;
}

interface SelectTriggerProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  children: React.ReactNode;
}

interface SelectContentProps {
  children: React.ReactNode;
  className?: string;
}

interface SelectItemProps {
  value: string;
  children: React.ReactNode;
  className?: string;
}

interface SelectValueProps {
  placeholder?: string;
}

const SelectContext = React.createContext<{
  value?: string;
  onValueChange?: (value: string) => void;
  open: boolean;
  setOpen: (open: boolean) => void;
}>({
  open: false,
  setOpen: () => {},
});

export function Select({ value, onValueChange, children }: SelectProps) {
  const [open, setOpen] = React.useState(false);

  return (
    <SelectContext.Provider value={{ value, onValueChange, open, setOpen }}>
      <div className="relative">
        {children}
      </div>
    </SelectContext.Provider>
  );
}

export function SelectTrigger({ className, children, ...props }: SelectTriggerProps) {
  const { open, setOpen } = React.useContext(SelectContext);

  return (
    <button
      type="button"
      className={cn(
        "flex h-10 w-full items-center justify-between rounded-md border border-slate-200 bg-white px-3 py-2 text-sm ring-offset-white placeholder:text-slate-500 focus:outline-none focus:ring-2 focus:ring-slate-950 focus:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 dark:border-slate-800 dark:bg-slate-950 dark:ring-offset-slate-950 dark:placeholder:text-slate-400 dark:focus:ring-slate-300",
        className
      )}
      onClick={() => setOpen(!open)}
      {...props}
    >
      {children}
      <svg
        width="15"
        height="15"
        viewBox="0 0 15 15"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
        className="h-4 w-4 opacity-50"
      >
        <path
          d="m4.93179 5.43179c-.20264-.20264-.20264-.53118 0-.73382.20264-.20264.53118-.20264.73382 0L7.5 6.43178l1.83439-1.73381c.20264-.20264.53118-.20264.73382 0 .20264.20264.20264.53118 0 .73382L8.23179 7.5l1.83382 1.93179c.20264.20264.20264.53118 0 .73382-.20264.20264-.53118.20264-.73382 0L7.5 8.43179l-1.83179 1.83382c-.20264.20264-.53118.20264-.73382 0-.20264-.20264-.20264-.53118 0-.73382L6.76821 7.5 4.93179 5.43179Z"
          fill="currentColor"
          fillRule="evenodd"
          clipRule="evenodd"
        />
      </svg>
    </button>
  );
}

export function SelectContent({ children, className }: SelectContentProps) {
  const { open, setOpen } = React.useContext(SelectContext);

  if (!open) return null;

  return (
    <>
      <div
        className="fixed inset-0 z-40"
        onClick={() => setOpen(false)}
      />
      <div className={cn(
        "absolute top-full left-0 z-50 mt-1 max-h-60 w-full overflow-auto rounded-md border border-slate-200 bg-white py-1 text-base shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none dark:border-slate-800 dark:bg-slate-950",
        className
      )}>
        {children}
      </div>
    </>
  );
}

export function SelectItem({ value, children, className }: SelectItemProps) {
  const { onValueChange, setOpen } = React.useContext(SelectContext);

  return (
    <div
      className={cn(
        "relative cursor-pointer select-none py-2 px-3 text-sm outline-none hover:bg-slate-100 focus:bg-slate-100 dark:hover:bg-slate-800 dark:focus:bg-slate-800",
        className
      )}
      onClick={() => {
        onValueChange?.(value);
        setOpen(false);
      }}
    >
      {children}
    </div>
  );
}

export function SelectValue({ placeholder }: SelectValueProps) {
  const { value } = React.useContext(SelectContext);
  
  return <span>{value || placeholder}</span>;
}