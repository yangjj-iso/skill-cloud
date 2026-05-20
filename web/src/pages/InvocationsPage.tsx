import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Invocation, listInvocations } from "../api";

export function InvocationsPage() {
  const [invocations, setInvocations] = useState<Invocation[] | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    listInvocations().then(setInvocations).catch((e) => setErr(String(e)));
  }, []);

  if (err) return <div className="error">{err}</div>;
  if (!invocations) return <div className="muted">Loading…</div>;

  return (
    <>
      <h2>Invocations</h2>
      <p className="muted">
        Most-recent <b>{invocations.length}</b> calls across every skill in your org.
      </p>
      <div className="card" style={{ padding: 0 }}>
        <table>
          <thead>
            <tr>
              <th>When</th>
              <th>Skill</th>
              <th>Status</th>
              <th>Latency</th>
              <th>Bytes in / out</th>
              <th>Caller IP</th>
            </tr>
          </thead>
          <tbody>
            {invocations.length === 0 ? (
              <tr>
                <td colSpan={6} className="muted">
                  No invocations yet.
                </td>
              </tr>
            ) : (
              invocations.map((inv, i) => (
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
                  <td className="muted">
                    {inv.input_bytes} / {inv.output_bytes}
                  </td>
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
