import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { listSkills, Skill } from "../api";

export function SkillsPage() {
  const [skills, setSkills] = useState<Skill[] | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    listSkills().then(setSkills).catch((e) => setErr(String(e)));
  }, []);

  if (err) return <div className="error">{err}</div>;
  if (!skills) return <div className="muted">Loading…</div>;

  return (
    <>
      <h2>Skills</h2>
      <div className="card" style={{ padding: 0 }}>
        <table>
          <thead>
            <tr>
              <th>Skill</th>
              <th>Version</th>
              <th>Runtime</th>
              <th>Description</th>
            </tr>
          </thead>
          <tbody>
            {skills.length === 0 ? (
              <tr>
                <td colSpan={4} className="muted">
                  No skills yet. Publish one with the <code>skill</code> CLI.
                </td>
              </tr>
            ) : (
              skills.map((s) => (
                <tr key={`${s.namespace}/${s.name}/${s.version}`}>
                  <td>
                    <Link to={`/skills/${s.namespace}/${s.name}`}>
                      {s.namespace}/{s.name}
                    </Link>
                  </td>
                  <td>
                    <code>{s.version}</code>
                  </td>
                  <td>{s.runtime?.type ?? "—"}</td>
                  <td className="muted">{s.description ?? ""}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </>
  );
}
