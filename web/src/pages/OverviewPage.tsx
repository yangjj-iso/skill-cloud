import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { getOverview, Overview } from "../api";

export function OverviewPage() {
  const [data, setData] = useState<Overview | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    getOverview()
      .then((d) => {
        if (!cancelled) setData(d);
      })
      .catch((e) => {
        if (!cancelled) setErr(String(e));
      });
    return () => {
      cancelled = true;
    };
  }, []);

  if (err) return <div className="error">{err}</div>;
  if (!data) return <div className="muted">Loading…</div>;

  return (
    <>
      <h2>Overview</h2>
      <div className="kpi">
        <div className="card">
          <div className="label">Skills</div>
          <div className="value">{data.skills_total}</div>
        </div>
        <div className="card">
          <div className="label">Invocations (total)</div>
          <div className="value">{data.invocations_total.toLocaleString()}</div>
        </div>
        <div className="card">
          <div className="label">Invocations (24h)</div>
          <div className="value">{data.invocations_24h.toLocaleString()}</div>
        </div>
        <div className="card">
          <div className="label">By runtime</div>
          <div className="value" style={{ fontSize: 18 }}>
            {Object.entries(data.skills_by_runtime)
              .map(([k, v]) => `${v} ${k}`)
              .join(" / ") || "—"}
          </div>
        </div>
      </div>

      <h3>Recent invocations</h3>
      <div className="card" style={{ padding: 0 }}>
        <table>
          <thead>
            <tr>
              <th>When</th>
              <th>Skill</th>
              <th>Status</th>
              <th>Latency</th>
              <th>Caller IP</th>
            </tr>
          </thead>
          <tbody>
            {data.recent.length === 0 ? (
              <tr>
                <td colSpan={5} className="muted">
                  No invocations yet.
                </td>
              </tr>
            ) : (
              data.recent.map((inv, i) => (
                <tr key={i}>
                  <td className="muted">{new Date(inv.started_at).toLocaleString()}</td>
                  <td>
                    {inv.namespace && inv.name ? (
                      <Link to={`/skills/${inv.namespace}/${inv.name}`}>
                        {inv.namespace}/{inv.name}
                      </Link>
                    ) : (
                      "—"
                    )}
                  </td>
                  <td className={`status-${inv.status}`}>{inv.status}</td>
                  <td>{inv.latency_ms} ms</td>
                  <td className="muted">{inv.caller_ip ?? "—"}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </>
  );
}
