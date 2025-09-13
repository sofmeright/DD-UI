// ui/src/types.ts

export type Host = {
    name: string;
    address?: string;
    groups?: string[];
};


export type ApiContainer = {
    name: string;
    image: string;
    state: string;
    status: string;
    owner?: string;
    ports?: any;
    labels?: Record<string, string>;
    updated_at?: string;
    created_ts?: string;
    ip_addr?: string;
    compose_project?: string;
    compose_service?: string;
    stack?: string | null;
};


export type IacEnvFile = { path: string; sops: boolean };


export type IacService = {
    id: number;
    stack_id: number;
    service_name: string;
    container_name?: string;
    image?: string;
    labels: Record<string, string>;
    env_keys: string[];
    env_files: IacEnvFile[];
    ports: any[];
    volumes: any[];
    deploy: Record<string, any>;
};


export type IacStack = {
    id: number;
    name: string; // stack_name
    scope_kind: string;
    scope_name: string;
    deploy_kind: "compose" | "script" | "unmanaged" | string;
    pull_policy?: string;
    sops_status: "all" | "partial" | "none" | string;
    iac_enabled: boolean; // Auto DevOps
    rel_path: string;
    compose?: string;
    services: IacService[] | null | undefined;
};


export type IacFileMeta = {
    role: string;
    rel_path: string;
    sops: boolean;
    sha256_hex: string;
    size_bytes: number;
    updated_at: string;
};


export type InspectOut = {
    id: string;
    name: string;
    image: string;
    state: string;
    health?: string;
    created: string;
    cmd?: string[];
    entrypoint?: string[];
    env?: Record<string, string>;
    labels?: Record<string, string>;
    restart_policy?: string;
    ports?: { published?: string; target?: string; protocol?: string }[];
    volumes?: { source?: string; target?: string; mode?: string; rw?: boolean }[];
    networks?: string[];
};


export type SessionResp = {
    user: null | {
        sub: string;
        email: string;
        name: string;
        picture?: string;
    };
};


export type MergedRow = {
    name: string;
    state: string;
    stack: string;
    imageRun?: string;
    imageIac?: string;
    created?: string;
    ip?: string;
    portsText?: string;
    owner?: string;
    drift?: boolean;
};


export type MergedStack = {
    name: string;
    drift: "in_sync" | "drift" | "unknown";
    iacEnabled: boolean;
    pullPolicy?: string;
    sops?: boolean;
    deployKind: string;
    rows: MergedRow[];
    hasIac: boolean;
    hasContent?: boolean;
};