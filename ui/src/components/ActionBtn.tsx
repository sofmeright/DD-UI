import { Button } from "@/components/ui/button";

export default function ActionBtn({
  title, onClick, icon: Icon, disabled=false
}: { title: string; onClick: ()=>void; icon: any; disabled?: boolean }) {
  return (
    <Button size="icon" variant="ghost" className="h-6 w-6 shrink-0" title={title} onClick={onClick} disabled={disabled}>
      <Icon className="h-3.5 w-3.5 text-slate-200" />
    </Button>
  );
}
