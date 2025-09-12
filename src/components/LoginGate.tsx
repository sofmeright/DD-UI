// ui/src/components/LoginGate.tsx
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

export default function LoginGate() {
  return (
    <div className="min-h-screen flex items-center justify-center bg-slate-950">
      <Card className="w-full max-w-sm bg-slate-900/60 border-slate-800">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <span className="font-black uppercase tracking-tight leading-none text-slate-200 select-none">
              <span className="bg-clip-text text-transparent bg-gradient-to-r from-brand to-sky-400">DDUI</span>
            </span>
            <Badge variant="outline">Community</Badge>
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-slate-300 text-sm">You're signed out. Continue to your identity provider to sign in.</p>
          <Button
            className="w-full bg-[#310937] hover:bg-[#2a0830] text-white"
            onClick={() => { window.location.replace("/auth/login"); }}
          >
            Continue to Sign in
          </Button>
          <p className="text-xs text-slate-500">
            If you get stuck, ensure your OIDC <code>RedirectURL</code> points back to
            <code> /auth/callback</code> and that cookies aren't blocked.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
