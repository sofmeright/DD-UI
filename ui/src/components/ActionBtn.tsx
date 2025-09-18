// ui/src/components/ActionBtn.tsx
import { Button } from "@/components/ui/button";
import { Loader2 } from "lucide-react";

type ActionBtnColor = "default" | "green" | "yellow" | "red" | "blue" | "orange";

const colorClasses: Record<ActionBtnColor, string> = {
  default: "text-slate-200 hover:text-slate-100",
  green: "text-emerald-400 hover:text-emerald-300",
  yellow: "text-yellow-400 hover:text-yellow-300", 
  red: "text-red-400 hover:text-red-300",
  blue: "text-blue-400 hover:text-blue-300",
  orange: "text-orange-400 hover:text-orange-300"
};

export default function ActionBtn({
  title, onClick, icon: Icon, disabled=false, color="default", loading=false
}: { title: string; onClick: ()=>void; icon: any; disabled?: boolean; color?: ActionBtnColor; loading?: boolean }) {
  const isDisabled = disabled || loading;
  
  return (
    <Button 
      size="icon" 
      variant="ghost" 
      className={`h-6 w-6 shrink-0 ${loading ? 'animate-pulse' : ''}`} 
      title={loading ? `${title}...` : title} 
      onClick={onClick} 
      disabled={isDisabled}
    >
      {loading ? (
        <Loader2 className={`h-3.5 w-3.5 animate-spin ${colorClasses[color]}`} />
      ) : (
        <Icon className={`h-3.5 w-3.5 ${colorClasses[color]} ${isDisabled ? 'opacity-50' : ''}`} />
      )}
    </Button>
  );
}
