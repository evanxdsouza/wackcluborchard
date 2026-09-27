import { h, Fragment, useEffect, useState } from "../lib/sprout.js";
import { Link } from "../lib/router.js";
import { useApi, useEvents } from "../lib/api.js";
import { duration, timeAgo, shortSha, appURL } from "../lib/format.js";
import { Loading, ErrorBox, Pill, Card, Callout, cx, Tag } from "../ui/kit.js";
import { Icon } from "../ui/icons.js";
import { DeployLog } from "../ui/term.js";

export function DeployPage({ org, id }: { org: string; id: string }) {
  const r = useApi<any>(`/deploys/${id}`);
  const app = useApi<any>(r.data ? `/apps/${r.data.appId}` : null, [r.data?.appId]);
  const [, tick] = useState(0);
  useEvents([`deploy:${id}`], (e) => {
    if (e.type === "deploy.updated") r.set(() => e.data);
  });
  useEffect(() => {
    const t = setInterval(() => tick((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, []);
  if (r.error) return <div class="page-mid"><ErrorBox error={r.error} /></div>;
  if (!r.data) return <div class="page-mid"><Loading /></div>;
  const d = r.data;
  const live = d.status === "running" || d.status === "queued";
  return (
    <div class="page-mid">
      <Link href={`/o/${org}/apps/${d.appId}?tab=deploys`} class="back-link"><Icon name="arrow-left" size={14} /> {app.data?.name || "app"}</Link>
      <div class="page-head" style={{ marginBottom: 10 }}>
        <h1 class="page-title" style={{ fontSize: 24 }}>Deploy #{d.number}</h1>
        <Pill status={d.status === "running" ? "deploying" : d.status} />
        <span class="muted" style={{ fontSize: 13.5 }}>{d.kind === "rollback" ? d.message : d.kind === "build" ? "Build from source" : "Image deploy"} · {d.trigger} · {d.createdBy} · started {timeAgo(d.createdAt)} · {duration(d.createdAt, d.finishedAt)}</span>
      </div>
      <div class="row row-wrap" style={{ marginBottom: 16 }}>
        {d.commit ? <Tag mono><Icon name="commit" size={12} /> {shortSha(d.commit)}</Tag> : null}
        {d.message && d.kind !== "rollback" ? <Tag>{d.message}</Tag> : null}
        {d.image ? <Tag mono>{d.image}</Tag> : null}
      </div>
      <div class="step-bar">{d.steps.map((s: any) => <div key={s.name} class={cx("step-seg", s.status)} />)}</div>
      <div class="grid-2" style={{ gridTemplateColumns: "minmax(0, 300px) minmax(0, 1fr)", alignItems: "start" }}>
        <div class="steps">
          {d.steps.map((s: any, i: number) => (
            <div key={s.name} class={cx("step", s.status)}>
              <div class="step-icon">
                {s.status === "succeeded" ? <Icon name="check" size={13} stroke={2.5} /> : s.status === "failed" ? <Icon name="x" size={13} stroke={2.5} /> : s.status === "running" ? <span class="spinner sm" /> : <span style={{ fontSize: 11, fontWeight: 600 }}>{i + 1}</span>}
              </div>
              <span class="step-name">{s.name}</span>
              <span class="step-time">{s.startedAt ? duration(s.startedAt, s.finishedAt) : ""}</span>
            </div>
          ))}
        </div>
        <div class="col">
          {d.status === "failed" ? (
            <Callout kind="red" title="The deploy failed.">
              {d.error} Nothing was torn down: the previous version keeps running. Fix the problem and redeploy; there is nothing to recreate.
            </Callout>
          ) : d.status === "succeeded" ? (
            <Callout kind="green" title="Live.">
              Rolled out and passing health checks. {app.data?.domains?.[0] ? <a href={appURL(app.data.domains[0].host)} target="_blank" rel="noopener">Open {app.data.domains[0].host} ↗</a> : null}
            </Callout>
          ) : d.status === "queued" ? (
            <Callout kind="info" title="Queued.">All build slots are busy. This build starts on its own when one frees up; there is no need to retry.</Callout>
          ) : d.status === "superseded" ? (
            <Callout kind="info">A newer deploy replaced this one before it started.</Callout>
          ) : null}
          {d.kind === "build" || live || d.status === "failed" ? <DeployLog deployId={d.id} height={460} /> : null}
        </div>
      </div>
    </div>
  );
}
