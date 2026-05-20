import { BrowserRouter, NavLink, Outlet, Route, Routes, useNavigate } from "react-router-dom";
import { useEffect, useState } from "react";
import { setApiKey } from "./api";
import { OverviewPage } from "./pages/OverviewPage";
import { SkillsPage } from "./pages/SkillsPage";
import { SkillDetailPage } from "./pages/SkillDetailPage";
import { InvocationsPage } from "./pages/InvocationsPage";

function Sidebar() {
  const navigate = useNavigate();
  return (
    <aside className="sidebar">
      <h1>Skill Cloud</h1>
      <nav>
        <NavLink to="/" end>
          Overview
        </NavLink>
        <NavLink to="/skills">Skills</NavLink>
        <NavLink to="/invocations">Invocations</NavLink>
      </nav>
      <div style={{ marginTop: 24 }}>
        <button
          onClick={() => {
            setApiKey("");
            navigate("/login");
          }}
        >
          Sign out
        </button>
      </div>
    </aside>
  );
}

function Layout() {
  return (
    <div className="layout">
      <Sidebar />
      <main className="content">
        <Outlet />
      </main>
    </div>
  );
}

function LoginPage() {
  const navigate = useNavigate();
  const [key, setKey] = useState("");
  const [err, setErr] = useState<string | null>(null);
  return (
    <div style={{ padding: 64, maxWidth: 480, margin: "0 auto" }}>
      <h2>Sign in to Skill Cloud</h2>
      <p className="muted">
        Paste an API key (created via <code>POST /v1/auth/api_keys</code> or the bootstrap
        flow). The key is stored in <code>localStorage</code> and sent as the{" "}
        <code>Authorization: Bearer</code> header on every request.
      </p>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          setErr(null);
          if (!key.trim()) {
            setErr("API key is required.");
            return;
          }
          setApiKey(key.trim());
          // Probe an authenticated endpoint so the user gets immediate
          // feedback when the key is rejected.
          const res = await fetch("/v1/skills", {
            headers: { Authorization: `Bearer ${key.trim()}` },
          });
          if (!res.ok) {
            setApiKey("");
            setErr(`API rejected the key (HTTP ${res.status}). Double-check the value.`);
            return;
          }
          navigate("/");
        }}
      >
        <div style={{ display: "grid", gap: 12 }}>
          <input
            type="password"
            autoFocus
            placeholder="sc_live_..."
            value={key}
            onChange={(e) => setKey(e.target.value)}
            data-testid="api-key-input"
          />
          <button type="submit">Sign in</button>
          {err ? <div className="error">{err}</div> : null}
        </div>
      </form>
    </div>
  );
}

function RequireAuth({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate();
  useEffect(() => {
    if (!localStorage.getItem("skillcloud.apiKey")) {
      navigate("/login", { replace: true });
    }
  }, [navigate]);
  return <>{children}</>;
}

export function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route
          element={
            <RequireAuth>
              <Layout />
            </RequireAuth>
          }
        >
          <Route index element={<OverviewPage />} />
          <Route path="/skills" element={<SkillsPage />} />
          <Route path="/skills/:namespace/:name" element={<SkillDetailPage />} />
          <Route path="/invocations" element={<InvocationsPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
