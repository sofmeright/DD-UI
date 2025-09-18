// ui/src/utils/metrics.ts
import { ApiContainer, IacService, IacStack } from "@/types";


const OK_STATES = new Set(["running", "created", "restarting", "healthy", "up"]);


export function isBadState(state?: string) {
    const s = (state || "").toLowerCase();
    if (!s) return false;
    for (const ok of OK_STATES) if (s.includes(ok)) return false;
    return true;
}

export function computeHostMetrics(runtime: ApiContainer[], iac: IacStack[]) {
    const rtByStack = new Map<string, ApiContainer[]>();
    for (const c of runtime) {
        const key = (c.compose_project || c.stack || "(none)").trim() || "(none)";
        if (!rtByStack.has(key)) rtByStack.set(key, []);
        rtByStack.get(key)!.push(c);
    }
    const iacByName = new Map<string, IacStack>();
    for (const s of iac) iacByName.set(s.name, s);

    const names = new Set<string>([...rtByStack.keys(), ...iacByName.keys()]);

    let stacks = 0;
    let containers = runtime.length;
    let drift = 0;
    let errors = 0;

    for (const c of runtime) if (isBadState(c.state)) errors++;

    for (const sname of names) {
        stacks++;
        const rcs = rtByStack.get(sname) || [];
        const is = iacByName.get(sname);
        const services: IacService[] = Array.isArray(is?.services) ? (is!.services as IacService[]) : [];
        const hasIac = !!is && (services.length > 0 || !!is.compose);
        let stackDrift = false;


        const desiredImageFor = (c: ApiContainer): string | undefined => {
            if (!is || services.length === 0) return undefined;
            const svc = services.find(x =>
                (c.compose_service && x.service_name === c.compose_service) ||
                (x.container_name && x.container_name === c.name)
            );
            return svc?.image || undefined;
        };

        for (const c of rcs) {
            const desired = desiredImageFor(c);
            if (desired && desired.trim() && desired.trim() !== (c.image || "").trim()) {
                stackDrift = true; break;
            }
        }
        if (!stackDrift && is && services.length > 0) {
            for (const svc of services) {
                const match = rcs.some(c =>
                    (c.compose_service && svc.service_name === c.compose_service) ||
                    (svc.container_name && c.name === svc.container_name)
                );
                if (!match) { stackDrift = true; break; }
            }
        }
        if (!rcs.length && hasIac && services.length > 0) stackDrift = true;
        if (stackDrift) drift++;
    }

    return { stacks, containers, drift, errors };
}