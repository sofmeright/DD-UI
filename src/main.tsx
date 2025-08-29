// src/main.tsx (high level)
import React, { useEffect, useState } from "react";
import ReactDOM from "react-dom/client";
import SignedOut from "./SignedOut";
import App from "./App";
import { getSession } from "./session";

function Root() {
  const [ready, setReady] = useState(false);
  const [authed, setAuthed] = useState(false);

  useEffect(() => {
    getSession().then(s => { setAuthed(!!s); setReady(true); });
  }, []);

  if (!ready) return null;
  return authed ? <App/> : <SignedOut/>;
}

ReactDOM.createRoot(document.getElementById("root")!).render(<Root />);