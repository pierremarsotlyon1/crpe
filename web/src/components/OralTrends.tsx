import type { components } from "@crpe/api-client";
import { dateLabel } from "../lib/model";

type Oral = components["schemas"]["OralSession"];
type Point = { session: string; date: string; status: "validated" | "to_work" };
export function OralTrends({ sessions }: { sessions: Oral[] }) {
  const series = new Map<string, Point[]>();
  for (const s of [...sessions].sort((a, b) =>
    a.createdAt.localeCompare(b.createdAt),
  )) {
    for (const item of s.feedback) {
      const values = series.get(item.criterion) ?? [];
      values.push({ session: s.id, date: s.createdAt, status: item.status });
      series.set(item.criterion, values);
    }
  }
  if (!series.size) return null;
  return (
    <section className="section">
      <h2>Progression par critère</h2>
      <div className="tiles">
        {Array.from(series, ([name, history]) => {
          const recent = history.slice(-8);
          const points = recent.map((p, i) => ({
            x: 12 + (i * 192) / Math.max(1, recent.length - 1),
            y: p.status === "validated" ? 12 : 50,
            ...p,
          }));
          return (
            <article key={name} className="panel">
              <h3>{name}</h3>
              {recent.length > 1 && (
                <svg role="img" viewBox="0 0 216 62" className="trend">
                  <title>
                    {name} · évolution des {recent.length} dernières évaluations
                  </title>
                  <path
                    d="M12 12H204M12 50H204"
                    fill="none"
                    stroke="var(--line)"
                  />
                  <polyline
                    points={points.map((p) => `${p.x},${p.y}`).join(" ")}
                    fill="none"
                    stroke="var(--ink)"
                    strokeWidth="2"
                  />
                  {points.map((p) => (
                    <circle
                      key={p.session}
                      cx={p.x}
                      cy={p.y}
                      r="4"
                      fill={
                        p.status === "validated" ? "var(--ink)" : "var(--ochre)"
                      }
                    />
                  ))}
                </svg>
              )}
              <ul className="plain-list">
                {recent.map((p) => (
                  <li key={p.session}>
                    <span>{dateLabel(p.date)}</span>
                    <span
                      className={`status ${p.status === "validated" ? "validated" : ""}`}
                    >
                      {p.status === "validated" ? "Validé" : "À travailler"}
                    </span>
                  </li>
                ))}
              </ul>
            </article>
          );
        })}
      </div>
    </section>
  );
}
