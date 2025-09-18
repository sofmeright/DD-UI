// ui/src/utils/format.ts
export function formatDT(s?: string) {
    if (!s) return "—";
    const d = new Date(s);
    if (isNaN(d.getTime())) return s;
    return d.toLocaleString();
}
    
export function formatPortsLines(ports: any): string[] {
    const arr: any[] =
        Array.isArray(ports) ? ports :
            (ports && Array.isArray(ports.ports)) ? ports.ports : [];
    const lines: string[] = [];
    for (const p of arr) {
        const ip = p.IP || p.Ip || p.ip || "";
        const pub = p.PublicPort ?? p.publicPort;
        const priv = p.PrivatePort ?? p.privatePort;
        const typ = (p.Type ?? p.type ?? "").toString().toLowerCase() || "tcp";
        if (priv) {
            const left = pub ? `${ip ? ip + ":" : ""}${pub}` : "";
            lines.push(`${left ? left + " → " : ""}${priv}/${typ}`);
        }
    }
    return lines;
}