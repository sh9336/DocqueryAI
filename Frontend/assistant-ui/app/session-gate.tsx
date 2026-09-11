"use client";

import { createContext, useCallback, useContext, useEffect, useRef, useState } from "react";
import {
  ApiError,
  acquireSession,
  getSessionStatus,
  releaseSession,
  sendHeartbeat,
  type SessionLimits,
  type SessionStatus,
} from "@/lib/api";

type GateState = "connecting" | "ready" | "full" | "unreachable" | "reconnecting";

interface SessionContextValue {
  limits: SessionLimits;
  status: SessionStatus | null;
  refreshStatus: () => Promise<void>;
}

const SessionContext = createContext<SessionContextValue | null>(null);

/** Access session limits/usage from anywhere inside <SessionGate>. */
export function useSession(): SessionContextValue {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error("useSession() must be used inside <SessionGate>");
  return ctx;
}

const FULL_RETRY_MS = 20_000;

export default function SessionGate({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<GateState>("connecting");
  const [limits, setLimits] = useState<SessionLimits | null>(null);
  const [status, setStatus] = useState<SessionStatus | null>(null);
  const heartbeatRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const refreshStatus = useCallback(async () => {
    try {
      setStatus(await getSessionStatus());
    } catch {
      // Non-fatal — quota UI just won't update this cycle.
    }
  }, []);

  const connect = useCallback(async () => {
    setState((prev) => (prev === "ready" ? "reconnecting" : "connecting"));
    try {
      const acquired = await acquireSession();
      setLimits(acquired);
      setState("ready");
      refreshStatus();
    } catch (err) {
      if (err instanceof ApiError && err.code === "demo_full") {
        setState("full");
      } else {
        setState("unreachable");
      }
    }
  }, [refreshStatus]);

  // Initial connect on mount. Guarded against React Strict Mode's
  // dev-only double-invoke of effects — without this, a single page load
  // fires acquireSession() twice in quick succession, and the second call
  // gets rejected by the per-IP session cap, making the app look broken
  // for one person on one tab.
  const didInit = useRef(false);
  useEffect(() => {
    if (didInit.current) return;
    didInit.current = true;
    connect();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Auto-retry while at capacity or unreachable.
  useEffect(() => {
    if (state !== "full" && state !== "unreachable") return;
    const t = setTimeout(connect, FULL_RETRY_MS);
    return () => clearTimeout(t);
  }, [state, connect]);

  // Heartbeat while ready; drop back to reconnecting if the lease expired.
  useEffect(() => {
    if (state !== "ready" || !limits) return;
    const intervalMs = Math.max(limits.heartbeat_seconds, 5) * 1000;
    heartbeatRef.current = setInterval(async () => {
      const alive = await sendHeartbeat();
      if (!alive) {
        if (heartbeatRef.current) clearInterval(heartbeatRef.current);
        connect();
      }
    }, intervalMs);
    return () => {
      if (heartbeatRef.current) clearInterval(heartbeatRef.current);
    };
  }, [state, limits, connect]);

  // Release the lease on tab close so the slot frees immediately instead of
  // waiting out the full inactivity timeout.
  useEffect(() => {
    const onUnload = () => releaseSession();
    window.addEventListener("beforeunload", onUnload);
    window.addEventListener("pagehide", onUnload);
    return () => {
      window.removeEventListener("beforeunload", onUnload);
      window.removeEventListener("pagehide", onUnload);
    };
  }, []);

  if (state === "full") {
    return (
      <GateScreen
        title="This demo is at capacity"
        message="Only a handful of visitors can use this live demo at once. A slot should free up shortly — this page will retry automatically."
        onRetry={connect}
      />
    );
  }

  if (state === "unreachable") {
    return (
      <GateScreen
        title="Waking up the server…"
        message="This runs on free-tier hosting, which can take up to a minute to wake up after being idle. Retrying automatically."
        onRetry={connect}
      />
    );
  }

  if (state === "connecting" || state === "reconnecting" || !limits) {
    return (
      <GateScreen
        title={state === "reconnecting" ? "Your session expired from inactivity" : "Connecting…"}
        message={state === "reconnecting" ? "Reconnecting you to a new session." : "Setting up your session."}
        spinner
      />
    );
  }

  return (
    <SessionContext.Provider value={{ limits, status, refreshStatus }}>
      {children}
    </SessionContext.Provider>
  );
}

function GateScreen({
  title,
  message,
  spinner,
  onRetry,
}: {
  title: string;
  message: string;
  spinner?: boolean;
  onRetry?: () => void;
}) {
  return (
    <div className="gate-screen">
      <div className="gate-card">
        {spinner ? (
          <div className="gate-spinner" aria-hidden />
        ) : (
          <div className="gate-icon" aria-hidden>
            <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="12" cy="12" r="10" />
              <line x1="12" y1="8" x2="12" y2="12" />
              <line x1="12" y1="16" x2="12.01" y2="16" />
            </svg>
          </div>
        )}
        <h1 className="gate-title">{title}</h1>
        <p className="gate-message">{message}</p>
        {onRetry && (
          <button className="gate-retry" onClick={onRetry}>
            Retry now
          </button>
        )}
      </div>
    </div>
  );
}
