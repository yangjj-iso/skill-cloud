import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  getSkill,
  getStats,
  invokeSkill,
  Invocation,
  listSkillLogs,
  Skill,
  Stats,
} from "../api";

export function SkillDetailPage() {
  const { namespace = "", name = "" } = useParams();
  const [skill, setSkill] = useState<Skill | null>(null);
  const [stats, setStats] = useState<Stats | null>(null);
  const [logs, setLogs] = useState<Invocation[] | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [input, setInput] = useState("{}");
  const [output, setOutput] = useState<string | null>(null);
  const [invoking, setInvoking] = useState(false);

  const reload = () => {
    Promise.all([
      getSkill(namespace, name),
      getStats(namespace, name),
      listSkillLogs(namespace, name, 25),
    ])
      .then(([sk, st, lg]) => {
        setSkill(sk);
        setStats(st);
        setLogs(lg);
      })
      .catch((e) => setErr(String(e)));
  };
  useEffect(reload, [namespace, name]);

  if (err) return <div className="error">{err}</div>;
  if (!skill || !stats || !logs) return <div className="muted">Loading…</div>;

  return (
    <>
      <h2>
        <Link to="/skills">Skills</Link> / {skill.namespace}/{skill.name}
      </h2>

      <div className="kpi">
        <div className="card">
          <div className="label">Version</div>
          <div className="value" style={{ fontSize: 18 }}>
            <code>{skill.version}</code>
          </div>
        </div>
        <div className="card">
          <div className="label">Runtime</div>
          <div className="value" style={{ fontSize: 18 }}>
            {skill.runtime?.type}
          </div>
        </div>
        <div className="card">
          <div className="label">Total calls</div>
          <div className="value">{stats.total}</div>
        </div>
        <div className="card">
          <div className="label">24h calls</div>
          <div className="value">{stats.last_24h}</div>
        </div>
      </div>

      <div className="card">
        <h3>Manifest</h3>
        <pre>{JSON.stringify(skill, null, 2)}</pre>
        <p className="muted">
          Runtime implementation details (image / entrypoint / url) are intentionally
          omitted; fetch <code>/v1/skills/{skill.namespace}/{skill.name}/runtime</code> as
          the owning org to see them.
        </p>
      </div>

      <div className="card">
        <h3>Invoke</h3>
        <textarea
          rows={4}
          style={{ width: "100%", fontFamily: "monospace" }}
          value={input}
          onChange={(e) => setInput(e.target.value)}
          data-testid="invoke-input"
        />
        <div className="row" style={{ marginTop: 8 }}>
          <button
            disabled={invoking}
            onClick={async () => {
              setInvoking(true);
              setOutput(null);
              try {
                const parsed = JSON.parse(input || "{}");
                const res = await invokeSkill(namespace, name, parsed);
                setOutput(JSON.stringify(res, null, 2));
                reload();
              } catch (e) {
                setOutput(String(e));
              } finally {
                setInvoking(false);
              }
            }}
          >
            {invoking ? "Invoking…" : "Invoke"}
          </button>
          <span className="muted">
            POST /v1/skills/{namespace}/{name}/invoke
          </span>
        </div>
        {output ? <pre style={{ marginTop: 12 }}>{output}</pre> : null}
      </div>

      <div className="card" style={{ padding: 0 }}>
        <h3 style={{ padding: "12px 20px 0" }}>Recent invocations</h3>
        <table>
          <thead>
            <tr>
              <th>When</th>
              <th>Status</th>
              <th>Latency</th>
              <th>Caller IP</th>
              <th>Error</th>
            </tr>
          </thead>
          <tbody>
            {logs.length === 0 ? (
              <tr>
                <td colSpan={5} className="muted">
                  No calls yet.
                </td>
              </tr>
            ) : (
              logs.map((inv, i) => (
                <tr key={i}>
                  <td className="muted">{new Date(inv.started_at).toLocaleString()}</td>
                  <td className={`status-${inv.status}`}>{inv.status}</td>
                  <td>{inv.latency_ms} ms</td>
                  <td className="muted">{inv.caller_ip ?? "—"}</td>
                  <td className="muted">{inv.error_message ?? ""}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </>
  );
}
