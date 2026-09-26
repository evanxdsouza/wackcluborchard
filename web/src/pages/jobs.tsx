import { h, Fragment, useEffect, useState } from "../lib/sprout.js";
import { Link, navigate, query } from "../lib/router.js";
import { useApi, useEvents, post, put, del, act, get } from "../lib/api.js";
import { timeAgo, duration, dateTime, plural } from "../lib/format.js";
import { Button, Loading, ErrorBox, Empty, Pill, Card, Field, Input, Select, Textarea, Toggle, confirm, Callout, Table, IconButton, cx, Segmented, Tag } from "../ui/kit.js";
import { Icon } from "../ui/icons.js";
import { OutputView } from "../ui/term.js";

function runStatus(s: string) {
  return s === "running" ? "deploying" : s;
}

export function JobsPage({ org }: { org: string }) {
  const r = useApi<any[]>(`/orgs/${org}/jobs`);
  return (
    <div class="page">
      <div class="page-head">
        <div>
          <h1 class="page-title">Jobs</h1>
          <div class="page-sub">Cron schedules and multi-step pipelines that run on the cluster, not on anyone's laptop.</div>
        </div>
        <div class="page-actions"><Button kind="primary" icon="plus" href={`/o/${org}/new/job`}>New job</Button></div>
      </div>
      {!r.data ? <Loading /> : r.data.length === 0 ? (
        <Empty icon="zap" title="No jobs yet" action={<Button kind="primary" icon="plus" href={`/o/${org}/new/job`}>New job</Button>}>
          Nightly cleanups, imports, migrations and reports. Steps run in order, each with its own logs and exit code.
        </Empty>
      ) : (
        <Table head={["Job", "Project", "Schedule", "Steps", "Last run", ""]}>
          {r.data.map((j) => (
            <tr key={j.id} class="clickable" onClick={() => navigate(`/o/${org}/jobs/${j.id}`)}>
              <td><div class="row"><Icon name="zap" size={15} class="muted" /><strong style={{ fontWeight: 600 }}>{j.name}</strong>{j.paused ? <Tag>paused</Tag> : null}</div></td>
              <td class="muted">{j.projectName}</td>
              <td><div>{j.scheduleText}</div>{j.nextRunAt && !j.paused ? <div class="muted" style={{ fontSize: 12 }}>next {timeAgo(j.nextRunAt)}</div> : null}</td>
              <td>{j.steps.length}</td>
              <td>{j.lastRun ? <div class="row"><Pill status={runStatus(j.lastRun.status)} label={j.lastRun.status[0].toUpperCase() + j.lastRun.status.slice(1)} /><span class="muted" style={{ fontSize: 12.5 }}>#{j.lastRun.number} · {timeAgo(j.lastRun.createdAt)}</span></div> : <span class="muted">never</span>}</td>
              <td style={{ textAlign: "right" }}>
                <Button size="sm" kind="secondary" icon="play" onClick={async (e: MouseEvent) => {
                  e.stopPropagation();
                  const run = await act(() => post(`/jobs/${j.id}/runs`));
                  if (run) navigate(`/o/${org}/runs/${run.id}`);
                }}>Run</Button>
              </td>
            </tr>
          ))}
        </Table>
      )}
    </div>
  );
}

export function JobPage({ org, id }: { org: string; id: string }) {
  const r = useApi<any>(`/jobs/${id}`);
  const [editing, setEditing] = useState(false);
  useEvents([`job:${id}`], (e) => {
    if (e.type === "run.updated") {
      r.set((d) => {
        const runs = d.runs.some((x: any) => x.id === e.data.id) ? d.runs.map((x: any) => (x.id === e.data.id ? e.data : x)) : [e.data, ...d.runs];
        return { ...d, runs };
      });
    }
  });
  if (r.error) return <div class="page-mid"><ErrorBox error={r.error} /></div>;
  if (!r.data) return <div class="page-mid"><Loading /></div>;
  const j = r.data;
  if (editing) return <JobEditor org={org} job={j} onDone={() => { setEditing(false); r.reload(); }} />;
  return (
    <div class="page-mid">
      <Link href={`/o/${org}/jobs`} class="back-link"><Icon name="arrow-left" size={14} /> Jobs</Link>
      <div class="page-head">
        <div>
          <h1 class="page-title" style={{ fontSize: 24 }}>{j.name}</h1>
          <div class="page-sub">{j.projectName} · {j.scheduleText}{j.nextRunAt && !j.paused ? ` · next ${timeAgo(j.nextRunAt)}` : ""} · concurrency: {j.concurrency}</div>
        </div>
        {j.canEdit ? (
          <div class="page-actions">
            <Button kind="ghost" icon={j.paused ? "play" : "pause"} onClick={async () => { if (await act(() => put(`/jobs/${id}`, { ...j, paused: !j.paused }), j.paused ? "Resumed" : "Paused")) r.reload(); }}>{j.paused ? "Resume" : "Pause"}</Button>
            <Button kind="secondary" icon="pencil" onClick={() => setEditing(true)}>Edit</Button>
            <Button kind="primary" icon="play" onClick={async () => { const run = await act(() => post(`/jobs/${id}/runs`)); if (run) navigate(`/o/${org}/runs/${run.id}`); }}>Run now</Button>
          </div>
        ) : null}
      </div>
      <div class="grid-2" style={{ gridTemplateColumns: "minmax(0,1fr) 300px", alignItems: "start" }}>
        <div>
          <div class="section-title" style={{ marginBottom: 10 }}>Runs</div>
          {j.runs.length === 0 ? <Empty icon="play" title="No runs yet">Run it now, or wait for the schedule.</Empty> : (
            <Table head={["", "Run", "Trigger", "Started", "Took"]}>
              {j.runs.map((run: any) => (
                <tr key={run.id} class="clickable" onClick={() => navigate(`/o/${org}/runs/${run.id}`)}>
                  <td style={{ width: 110 }}><Pill status={runStatus(run.status)} label={run.status[0].toUpperCase() + run.status.slice(1)} /></td>
                  <td><strong>#{run.number}</strong> <span class="muted" style={{ fontSize: 12.5 }}>{run.steps.map((s: any) => (s.status === "succeeded" ? "✓" : s.status === "failed" ? "✗" : "·")).join(" ")}</span></td>
                  <td><span class="tag">{run.trigger}</span> <span class="muted" style={{ fontSize: 12.5 }}>{run.createdBy}</span></td>
                  <td title={dateTime(run.createdAt)}>{timeAgo(run.createdAt)}</td>
                  <td class="mono" style={{ fontSize: 12.5 }}>{duration(run.startedAt || run.createdAt, run.finishedAt)}</td>
                </tr>
              ))}
            </Table>
          )}
        </div>
        <Card>
          <div class="panel-title" style={{ marginBottom: 10 }}>Pipeline</div>
          <div class="col" style={{ gap: 8 }}>
            {j.steps.map((s: any, i: number) => (
              <div key={i} class="row" style={{ alignItems: "flex-start" }}>
                <span class="count-badge" style={{ marginTop: 1 }}>{i + 1}</span>
                <div class="grow">
                  <div style={{ fontWeight: 500 }}>{s.name}</div>
                  <div class="muted mono" style={{ fontSize: 12 }}>{s.type === "script" ? s.lang : s.type === "app" ? "app command" : s.image}</div>
                </div>
              </div>
            ))}
          </div>
          {j.canEdit ? (
            <>
              <hr class="divider" />
              <Button kind="ghost" icon="trash" size="sm" onClick={async () => {
                if (await confirm({ title: `Delete ${j.name}?`, body: "Its run history goes too.", danger: true, confirm: "Delete job" })) {
                  if (await act(() => del(`/jobs/${id}`), "Job deleted")) navigate(`/o/${org}/jobs`);
                }
              }}>Delete job</Button>
            </>
          ) : null}
        </Card>
      </div>
    </div>
  );
}

const templates: Record<string, string> = {
  bash: "#!/usr/bin/env bash\nset -euo pipefail\n\necho \"hello from $(hostname)\"\n",
  python: "import os\n\nprint('hello from', os.uname().nodename)\n",
  node: "console.log('hello from', require('os').hostname());\n",
};

export function JobEditor({ org, job, onDone }: { org: string; job?: any; onDone?: () => void }) {
  const [projectId, setProjectId] = useState(job?.projectId || query().get("project") || "");
  const projects = useApi<any[]>(job ? null : `/orgs/${org}/overview`);
  const apps = useApi<any>(projectId ? `/projects/${projectId}` : null, [projectId]);
  const [name, setName] = useState(job?.name || "");
  const [schedule, setSchedule] = useState(job?.schedule || "");
  const [concurrency, setConcurrency] = useState(job?.concurrency || "skip");
  const [steps, setSteps] = useState<any[]>(job?.steps?.map((s: any) => ({ ...s })) || [{ name: "step 1", type: "script", lang: "bash", source: templates.bash }]);
  useEffect(() => {
    if (!projectId && projects.data?.length) setProjectId(projects.data.find((p) => p.canEdit)?.id || "");
  }, [projects.data]);
  const setStep = (i: number, patch: any) => setSteps(steps.map((s, j) => (j === i ? { ...s, ...patch } : s)));
  const save = async () => {
    const body = { name, schedule, concurrency, steps, paused: job?.paused || false };
    const res = await act(() => (job ? put(`/jobs/${job.id}`, body) : post(`/projects/${projectId}/jobs`, body)), job ? "Saved" : "Job created");
    if (res) {
      if (onDone) onDone();
      else navigate(`/o/${org}/jobs/${res.id}`);
    }
  };
  const swallow = steps.some((s) => s.type === "script" && /\|\|\s*true\s*$/m.test(s.source || ""));
  return (
    <div class="page-mid">
      <Link href={job ? `/o/${org}/jobs/${job.id}` : `/o/${org}/jobs`} class="back-link" onClick={(e: Event) => { if (onDone) { e.preventDefault(); onDone(); } }}><Icon name="arrow-left" size={14} /> {job ? job.name : "Jobs"}</Link>
      <div class="page-head"><h1 class="page-title" style={{ fontSize: 24 }}>{job ? "Edit job" : "New job"}</h1></div>
      <div class="col gap-lg">
        <Card class="form-card">
          <div class="form-grid">
            <Field label="Name"><Input value={name} onInput={(v) => setName(v.toLowerCase().replace(/[^a-z0-9-]/g, "-"))} placeholder="nightly-cleanup" mono autofocus /></Field>
            {!job ? (
              <Field label="Project">
                <Select value={projectId} onChange={setProjectId} options={(projects.data || []).filter((p) => p.canEdit).map((p) => ({ value: p.id, label: `${p.icon} ${p.name}` }))} />
              </Field>
            ) : <Field label="Project"><Input value={job.projectName} onInput={() => {}} disabled /></Field>}
            <Field label="Schedule" hint="Standard cron in UTC, or @hourly / @daily. Empty means it only runs when triggered.">
              <Input value={schedule} onInput={setSchedule} placeholder="0 3 * * *" mono />
            </Field>
            <Field label="If the previous run is still going">
              <Select value={concurrency} onChange={setConcurrency} options={[{ value: "skip", label: "Skip the new run" }, { value: "queue", label: "Queue it" }, { value: "allow", label: "Run both" }]} />
            </Field>
          </div>
          <div class="row row-wrap" style={{ gap: 6 }}>
            {[["@hourly", "Hourly"], ["0 3 * * *", "Daily 03:00"], ["*/15 * * * *", "Every 15 min"], ["0 9 * * 1-5", "Weekdays 09:00"], ["", "Manual only"]].map(([v, l]) => (
              <button key={l} type="button" class={cx("tag", schedule === v && "active")} style={{ cursor: "pointer", borderColor: schedule === v ? "var(--accent)" : undefined }} onClick={() => setSchedule(v)}>{l}</button>
            ))}
          </div>
        </Card>
        {steps.map((s, i) => (
          <Card key={i} class="form-card">
            <div class="row">
              <span class="count-badge">{i + 1}</span>
              <Input value={s.name} onInput={(v) => setStep(i, { name: v })} placeholder="step name" />
              <Segmented value={s.type} onChange={(v) => setStep(i, { type: v, source: v === "script" ? s.source || templates.bash : s.source })} options={[{ id: "script", label: "Script", icon: "code" }, { id: "app", label: "App command", icon: "box" }, { id: "image", label: "Custom image", icon: "layers" }]} />
              <IconButton icon="chevron-up" title="Move up" onClick={() => i > 0 && setSteps([...steps.slice(0, i - 1), steps[i], steps[i - 1], ...steps.slice(i + 1)])} />
              <IconButton icon="trash" title="Remove step" onClick={() => setSteps(steps.filter((_, j) => j !== i))} />
            </div>
            {s.type === "script" ? (
              <>
                <div class="row">
                  <Segmented size="sm" value={s.lang || "bash"} onChange={(v) => setStep(i, { lang: v, source: !s.source || Object.values(templates).includes(s.source) ? templates[v] : s.source })} options={[{ id: "bash", label: "bash" }, { id: "python", label: "python" }, { id: "node", label: "node" }]} />
                  <span class="muted" style={{ fontSize: 12.5 }}>runs in <code>{{ bash: "debian:12-slim", python: "python:3.13-slim", node: "node:22-slim" }[s.lang as string || "bash"]}</code></span>
                </div>
                <Textarea value={s.source} onInput={(v) => setStep(i, { source: v })} rows={8} mono />
              </>
            ) : s.type === "app" ? (
              <div class="form-grid">
                <Field label="App" hint="Runs in the app's current image with its variables.">
                  <Select value={s.appId || ""} onChange={(v) => setStep(i, { appId: v })} options={[{ value: "", label: "Pick an app…" }, ...((apps.data?.apps || []).map((a: any) => ({ value: a.id, label: a.name })))]} />
                </Field>
                <Field label="Command"><Input value={s.command || ""} onInput={(v) => setStep(i, { command: v })} placeholder="npm run migrate" mono /></Field>
              </div>
            ) : (
              <div class="form-grid">
                <Field label="Image"><Input value={s.image || ""} onInput={(v) => setStep(i, { image: v })} placeholder="curlimages/curl:latest" mono /></Field>
                <Field label="Command"><Input value={s.command || ""} onInput={(v) => setStep(i, { command: v })} placeholder="curl -fsS https://example.com/ping" mono /></Field>
              </div>
            )}
          </Card>
        ))}
        {swallow ? <Callout kind="amber" title="A step ends in || true.">It always reports success, which turns a broken job into a green one. Make sure the job still fails on the thing that actually matters.</Callout> : null}
        <Callout kind="info">Script images are deliberately small: <code>debian:12-slim</code> has no <code>curl</code>. Install what you need on the first line, or use a custom image that has it.</Callout>
        <div class="row">
          <Button kind="ghost" icon="plus" onClick={() => setSteps([...steps, { name: `step ${steps.length + 1}`, type: "script", lang: "bash", source: templates.bash }])}>Add step</Button>
          <span class="grow" />
          <Button kind="primary" icon="save" onClick={save} disabled={!name || !projectId || !steps.length}>{job ? "Save job" : "Create job"}</Button>
        </div>
      </div>
    </div>
  );
}

export function NewJobPage({ org }: { org: string }) {
  return <JobEditor org={org} />;
}

export function RunPage({ org, id }: { org: string; id: string }) {
  const r = useApi<any>(`/runs/${id}`);
  const [open, setOpen] = useState<Record<number, boolean>>({});
  const [, tick] = useState(0);
  useEvents([`run:${id}`], (e) => {
    if (e.type === "run.updated") r.set((d) => ({ ...d, ...e.data }));
  });
  useEffect(() => {
    const t = setInterval(() => tick((n) => n + 1), 1000);
    return () => clearInterval(t);
  }, []);
  if (r.error) return <div class="page-mid"><ErrorBox error={r.error} /></div>;
  if (!r.data) return <div class="page-mid"><Loading /></div>;
  const run = r.data;
  const live = run.status === "running" || run.status === "queued";
  const isOpen = (i: number, s: any) => open[i] ?? (s.status !== "pending" && s.status !== "skipped");
  return (
    <div class="page-mid" style={{ maxWidth: 944 }}>
      <Link href={`/o/${org}/jobs/${run.jobId}`} class="back-link"><Icon name="arrow-left" size={14} /> {run.jobName}</Link>
      <div class="row" style={{ gap: 10 }}>
        <h1 class="page-title" style={{ fontSize: 24 }}>Run #{run.number}</h1>
        <Pill status={runStatus(run.status)} label={run.status[0].toUpperCase() + run.status.slice(1)} />
        <span class="grow" />
        {live ? <Button size="sm" kind="secondary" icon="square" onClick={() => act(() => post(`/runs/${id}/cancel`), "Cancelling")}>Cancel</Button> : null}
      </div>
      <div class="muted" style={{ fontSize: 13.5, margin: "4px 0 18px", display: "flex", gap: 14 }}>
        <span>{run.trigger[0].toUpperCase() + run.trigger.slice(1)}{run.createdBy ? " · " + run.createdBy : ""}</span>
        <span>started {timeAgo(run.startedAt || run.createdAt)}</span>
        <span>{duration(run.startedAt || run.createdAt, run.finishedAt)}</span>
      </div>
      <Card class="run-card">
        <div class="col" style={{ gap: 18 }}>
          {run.steps.map((s: any, i: number) => (
            <div key={i}>
              <span class="run-tab"><Icon name="file" size={11} /> {s.type === "app" ? "app" : s.type === "image" ? "image" : "script"}</span>
              <div class="run-step">
                <div class="run-step-head" onClick={() => setOpen({ ...open, [i]: !isOpen(i, s) })}>
                  <span class={cx("step-check", s.status)}>{s.status === "succeeded" ? <Icon name="check" size={12} stroke={2.5} /> : s.status === "failed" ? <Icon name="x" size={12} stroke={2.5} /> : null}</span>
                  <span class="run-step-name">{s.name}</span>
                  <span class="run-step-image">{s.image}</span>
                  <span class="run-step-dur">{s.startedAt ? duration(s.startedAt, s.finishedAt) : s.status}</span>
                </div>
              </div>
              {isOpen(i, s) ? (
                <>
                  <div class="run-out"><OutputView text={s.output} onRefresh={r.reload} height={420} /></div>
                  {s.exitCode != null ? <div class="run-exit">exit code {s.exitCode}</div> : null}
                </>
              ) : null}
            </div>
          ))}
        </div>
      </Card>
    </div>
  );
}

export { get, Toggle, plural };
