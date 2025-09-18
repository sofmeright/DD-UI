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
    container_id?: string;  // Alternative ID field
    name: string;
    image: string;
    state: string;
    status?: string;  // Container status text
    health?: string;
    created?: string;
    created_ts?: string;  // Alternative created timestamp
    created_at?: string;  // Another created field variant
    cmd?: string[];
    entrypoint?: string[];
    env?: string[];  // Array of "KEY=VALUE" strings from Docker
    labels?: Record<string, string>;
    restart_policy?: string;
    ports?: any[];  // Can be various formats from Docker
    volumes?: { source?: string; target?: string; mode?: string; rw?: boolean }[];
    mounts?: any[];  // Docker mounts array
    networks?: string[] | Record<string, any>;  // Can be array of names or map of network details
    ip_addr?: string;  // IP address
    owner?: string;  // Owner of container
    compose_project?: string;  // Docker Compose project name
    compose_service?: string;  // Docker Compose service name
    updated_at?: string;  // Last update time
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
    status?: string; // Docker status with detailed info (e.g., "Up 2 hours", "Exited (0) 5 minutes ago")
    stack: string;
    imageRun?: string;
    imageIac?: string;
    created?: string;
    ip?: string;
    portsText?: string; // Keep for backward compatibility
    ports?: any; // Raw ports data for PortLinks component
    owner?: string;
    drift?: boolean;
};


export type MergedStack = {
    name: string;
    drift: "in_sync" | "drift" | "unknown";
    iacEnabled: boolean;
    autoDevOps?: boolean;  // Separate property for Auto DevOps toggle (effective_auto_devops from API)
    pullPolicy?: string;
    sops?: boolean;
    deployKind: string;
    rows: MergedRow[];
    hasIac: boolean;
    hasContent?: boolean;
};