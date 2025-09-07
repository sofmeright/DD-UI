import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";


export default function MetricCard({
    title, value, icon: Icon, accent = false,
}: { title: string; value: React.ReactNode; icon: any; accent?: boolean }) {
    return (
        <Card className={`border-slate-800 ${accent ? "bg-slate-900/40 border-brand/40" : "bg-slate-900/40"}`}>
            <CardHeader className="flex flex-row items-center justify-between pb-2">
                <CardTitle className="text-sm font-medium text-slate-300">{title}</CardTitle>
                <Icon className="h-4 w-4 text-slate-400" />
            </CardHeader>
            <CardContent>
                <div className={`text-2xl font-extrabold ${accent ? "text-brand" : "text-white"}`}>{value}</div>
            </CardContent>
        </Card>
    );
}