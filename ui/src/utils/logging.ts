// Frontend logging utility that mirrors backend DD_UI_LOG_LEVEL system
// Maps to the same log levels as src/api/main.go

type LogLevel = "debug" | "info" | "warn" | "error" | "fatal";

const levelOrder: Record<LogLevel, number> = {
  debug: 0,
  info: 1,
  warn: 2,
  error: 3,
  fatal: 4,
};

function getLogLevel(): LogLevel {
  // Check runtime config first (from config.js), then build-time env vars
  const runtimeLevel = (window as any)?.DD_UI_CONFIG?.LOG_LEVEL;
  const level = (
    runtimeLevel ||
    (import.meta.env.VITE_DD_UI_LOG_LEVEL as string) ||
    (import.meta.env.DD_UI_LOG_LEVEL as string) ||
    "info"
  ).toLowerCase();
  return (levelOrder[level as LogLevel] !== undefined) ? level as LogLevel : "info";
}

function shouldLog(level: LogLevel): boolean {
  const currentLevel = getLogLevel();
  const currentLevelNum = levelOrder[currentLevel];
  const targetLevelNum = levelOrder[level];
  return targetLevelNum >= currentLevelNum;
}

export function debugLog(message: string, ...args: any[]): void {
  if (shouldLog("debug")) {
    console.log(`DEBUG: ${message}`, ...args);
  }
}

// Always log the current log level on first import for debugging
const runtimeConfig = (window as any)?.DD_UI_CONFIG;
console.log(`DD-UI Frontend Log Level: ${getLogLevel()}`, {
  runtime_config: runtimeConfig?.LOG_LEVEL,
  vite_env: import.meta.env.VITE_DD_UI_LOG_LEVEL,
  dd_ui_env: import.meta.env.DD_UI_LOG_LEVEL,
  config_generated: runtimeConfig?.GENERATED_AT
});

export function infoLog(message: string, ...args: any[]): void {
  if (shouldLog("info")) {
    console.log(`INFO: ${message}`, ...args);
  }
}

export function warnLog(message: string, ...args: any[]): void {
  if (shouldLog("warn")) {
    console.warn(`WARN: ${message}`, ...args);
  }
}

export function errorLog(message: string, ...args: any[]): void {
  if (shouldLog("error")) {
    console.error(`ERROR: ${message}`, ...args);
  }
}

export function fatalLog(message: string, ...args: any[]): void {
  console.error(`FATAL: ${message}`, ...args);
}