// ui/src/components/Fact.tsx
export default function Fact({ label, value }: { label: string; value: React.ReactNode }) {
    return (
      <div className="flex items-start gap-3">
        <div className="shrink-0 text-xs uppercase tracking-wide text-slate-400 w-28">{label}</div>
        <div className="text-slate-300 min-w-0 break-words">{value}</div>
      </div>
    );
  }