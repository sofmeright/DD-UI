// src/SignedOut.tsx
export default function SignedOut() {
    return (
      <div className="min-h-screen grid place-items-center bg-slate-950 text-slate-100">
        <div className="p-8 rounded-2xl border border-slate-800 bg-slate-900/60">
          <h1 className="text-2xl font-extrabold mb-2">DD<span className="text-[#74ecbe]">UI</span></h1>
          <p className="text-slate-400 mb-4">Sign in to continue</p>
          <a href="/auth/login" className="inline-flex px-4 py-2 rounded-lg bg-[#74ecbe] text-slate-900 font-semibold">Sign in</a>
        </div>
      </div>
    );
  }