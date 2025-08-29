import React from "react";

type User = { sub: string; email: string; name: string; picture?: string };
type AuthState = { user: User | null; loading: boolean };

export function useAuth() {
  const [state, setState] = React.useState<AuthState>({ user: null, loading: true });

  React.useEffect(() => {
    let alive = true;
    fetch("/api/session", { credentials: "include" })
      .then(async (res) => {
        if (res.status === 401) { if (alive) setState({ user: null, loading: false }); return; }
        const data = await res.json();
        if (alive) setState({ user: data.user ?? null, loading: false });
      })
      .catch(() => alive && setState({ user: null, loading: false }));
    return () => { alive = false; };
  }, []);

  return state;
}

export function AuthGate({ children }: { children: React.ReactNode }) {
  const { user, loading } = useAuth();

  if (loading) {
    return (
      <div className="min-h-screen grid place-items-center bg-slate-950 text-slate-200">
        <div className="animate-pulse text-slate-400">Loading…</div>
      </div>
    );
  }

  if (!user) return <LoginSplash />;

  return <>{children}</>;
}

function LoginSplash() {
  return (
    <div className="min-h-screen bg-slate-950 text-slate-100 flex items-center justify-center p-6">
      <div className="w-full max-w-md rounded-2xl border border-slate-800 bg-slate-900/60 p-6 text-center">
        <div className="mx-auto mb-4 h-12 w-12 rounded-2xl bg-slate-900 border border-slate-800 grid place-items-center shadow-[0_0_0_4px_rgba(116,236,190,0.12)]">
          <span className="font-black text-lg">
            DD<span className="bg-clip-text text-transparent bg-gradient-to-r from-[#74ecbe] to-[#60a5fa]">UI</span>
          </span>
        </div>
        <div className="text-xl font-bold mb-2">Designated Driver UI</div>
        <div className="text-sm text-slate-400 mb-6">Sign in with your identity provider to continue.</div>
        <a
          href="/login"
          className="inline-flex items-center justify-center rounded-md px-4 py-2 font-medium bg-[#74ecbe] text-slate-900 hover:bg-[#63d9ad]"
        >
          Sign in with SSO
        </a>
        <div className="mt-4 text-xs text-slate-500">Community Edition • Security-first by default</div>
      </div>
    </div>
  );
}