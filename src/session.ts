// src/session.ts
export type Session = { user: { email: string; name: string; picture?: string } };
export async function getSession(): Promise<Session | null> {
  const res = await fetch("/api/session", { credentials: "include" });
  if (res.status === 200) return res.json();
  return null;
}