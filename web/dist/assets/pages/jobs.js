import { h, Fragment, useEffect, useState } from "../lib/sprout.js";
import { Link, navigate, query } from "../lib/router.js";
import { useApi, useEvents, post, put, del, act, get } from "../lib/api.js";
import { timeAgo, duration, dateTime, plural } from "../lib/format.js";
import { Button, Loading, ErrorBox, Empty, Pill, Card, Field, Input, Select, Textarea, Toggle, confirm, Callout, Table, IconButton, cx, Segmented, Tag } from "../ui/kit.js";
import { Icon } from "../ui/icons.js";
import { OutputView } from "../ui/term.js";
function runStatus(s) {
    return s === "running" ? "deploying" : s;
}
export function JobsPage({ org }) {
    const r = useApi(`/orgs/${org}/jobs`);
    return (h("div", { class: "page" },
        h("div", { class: "page-head" },
            h("div", null,
                h("h1", { class: "page-title" }, "Jobs"),
                h("div", { class: "page-sub" }, "Cron schedules and multi-step pipelines that run on the cluster, not on anyone's laptop.")),
            h("div", { class: "page-actions" },
                h(Button, { kind: "primary", icon: "plus", href: `/o/${org}/new/job` }, "New job"))),
        !r.data ? h(Loading, null) : r.data.length === 0 ? (h(Empty, { icon: "zap", title: "No jobs yet", action: h(Button, { kind: "primary", icon: "plus", href: `/o/${org}/new/job` }, "New job") }, "Nightly cleanups, imports, migrations and reports. Steps run in order, each with its own logs and exit code.")) : (h(Table, { head: ["Job", "Project", "Schedule", "Steps", "Last run", ""] }, r.data.map((j) => (h("tr", { key: j.id, class: "clickable", onClick: () => navigate(`/o/${org}/jobs/${j.id}`) },
            h("td", null,
                h("div", { class: "row" },
                    h(Icon, { name: "zap", size: 15, class: "muted" }),
                    h("strong", { style: { fontWeight: 600 } }, j.name),
                    j.paused ? h(Tag, null, "paused") : null)),
            h("td", { class: "muted" }, j.projectName),
            h("td", null,
                h("div", null, j.scheduleText),
                j.nextRunAt && !j.paused ? h("div", { class: "muted", style: { fontSize: 12 } },
                    "next ",
                    timeAgo(j.nextRunAt)) : null),
            h("td", null, j.steps.length),
            h("td", null, j.lastRun ? h("div", { class: "row" },
                h(Pill, { status: runStatus(j.lastRun.status), label: j.lastRun.status[0].toUpperCase() + j.lastRun.status.slice(1) }),
                h("span", { class: "muted", style: { fontSize: 12.5 } },
                    "#",
                    j.lastRun.number,
                    " \u00B7 ",
                    timeAgo(j.lastRun.createdAt))) : h("span", { class: "muted" }, "never")),
            h("td", { style: { textAlign: "right" } },
                h(Button, { size: "sm", kind: "secondary", icon: "play", onClick: async (e) => {
                        e.stopPropagation();
                        const run = await act(() => post(`/jobs/${j.id}/runs`));
                        if (run)
                            navigate(`/o/${org}/runs/${run.id}`);
                    } }, "Run")))))))));
}
export function JobPage({ org, id }) {
    const r = useApi(`/jobs/${id}`);
    const [editing, setEditing] = useState(false);
    useEvents([`job:${id}`], (e) => {
        if (e.type === "run.updated") {
            r.set((d) => {
                const runs = d.runs.some((x) => x.id === e.data.id) ? d.runs.map((x) => (x.id === e.data.id ? e.data : x)) : [e.data, ...d.runs];
                return { ...d, runs };
            });
        }
    });
    if (r.error)
        return h("div", { class: "page-mid" },
            h(ErrorBox, { error: r.error }));
    if (!r.data)
        return h("div", { class: "page-mid" },
            h(Loading, null));
    const j = r.data;
    if (editing)
        return h(JobEditor, { org: org, job: j, onDone: () => { setEditing(false); r.reload(); } });
    return (h("div", { class: "page-mid" },
        h(Link, { href: `/o/${org}/jobs`, class: "back-link" },
            h(Icon, { name: "arrow-left", size: 14 }),
            " Jobs"),
        h("div", { class: "page-head" },
            h("div", null,
                h("h1", { class: "page-title", style: { fontSize: 24 } }, j.name),
                h("div", { class: "page-sub" },
                    j.projectName,
                    " \u00B7 ",
                    j.scheduleText,
                    j.nextRunAt && !j.paused ? ` · next ${timeAgo(j.nextRunAt)}` : "",
                    " \u00B7 concurrency: ",
                    j.concurrency)),
            j.canEdit ? (h("div", { class: "page-actions" },
                h(Button, { kind: "ghost", icon: j.paused ? "play" : "pause", onClick: async () => { if (await act(() => put(`/jobs/${id}`, { ...j, paused: !j.paused }), j.paused ? "Resumed" : "Paused"))
                        r.reload(); } }, j.paused ? "Resume" : "Pause"),
                h(Button, { kind: "secondary", icon: "pencil", onClick: () => setEditing(true) }, "Edit"),
                h(Button, { kind: "primary", icon: "play", onClick: async () => { const run = await act(() => post(`/jobs/${id}/runs`)); if (run)
                        navigate(`/o/${org}/runs/${run.id}`); } }, "Run now"))) : null),
        h("div", { class: "grid-2", style: { gridTemplateColumns: "minmax(0,1fr) 300px", alignItems: "start" } },
            h("div", null,
                h("div", { class: "section-title", style: { marginBottom: 10 } }, "Runs"),
                j.runs.length === 0 ? h(Empty, { icon: "play", title: "No runs yet" }, "Run it now, or wait for the schedule.") : (h(Table, { head: ["", "Run", "Trigger", "Started", "Took"] }, j.runs.map((run) => (h("tr", { key: run.id, class: "clickable", onClick: () => navigate(`/o/${org}/runs/${run.id}`) },
                    h("td", { style: { width: 110 } },
                        h(Pill, { status: runStatus(run.status), label: run.status[0].toUpperCase() + run.status.slice(1) })),
                    h("td", null,
                        h("strong", null,
                            "#",
                            run.number),
                        " ",
                        h("span", { class: "muted", style: { fontSize: 12.5 } }, run.steps.map((s) => (s.status === "succeeded" ? "✓" : s.status === "failed" ? "✗" : "·")).join(" "))),
                    h("td", null,
                        h("span", { class: "tag" }, run.trigger),
                        " ",
                        h("span", { class: "muted", style: { fontSize: 12.5 } }, run.createdBy)),
                    h("td", { title: dateTime(run.createdAt) }, timeAgo(run.createdAt)),
                    h("td", { class: "mono", style: { fontSize: 12.5 } }, duration(run.startedAt || run.createdAt, run.finishedAt)))))))),
            h(Card, null,
                h("div", { class: "panel-title", style: { marginBottom: 10 } }, "Pipeline"),
                h("div", { class: "col", style: { gap: 8 } }, j.steps.map((s, i) => (h("div", { key: i, class: "row", style: { alignItems: "flex-start" } },
                    h("span", { class: "count-badge", style: { marginTop: 1 } }, i + 1),
                    h("div", { class: "grow" },
                        h("div", { style: { fontWeight: 500 } }, s.name),
                        h("div", { class: "muted mono", style: { fontSize: 12 } }, s.type === "script" ? s.lang : s.type === "app" ? "app command" : s.image)))))),
                j.canEdit ? (h(Fragment, null,
                    h("hr", { class: "divider" }),
                    h(Button, { kind: "ghost", icon: "trash", size: "sm", onClick: async () => {
                            if (await confirm({ title: `Delete ${j.name}?`, body: "Its run history goes too.", danger: true, confirm: "Delete job" })) {
                                if (await act(() => del(`/jobs/${id}`), "Job deleted"))
                                    navigate(`/o/${org}/jobs`);
                            }
                        } }, "Delete job"))) : null))));
}
const templates = {
    bash: "#!/usr/bin/env bash\nset -euo pipefail\n\necho \"hello from $(hostname)\"\n",
    python: "import os\n\nprint('hello from', os.uname().nodename)\n",
    node: "console.log('hello from', require('os').hostname());\n",
};
export function JobEditor({ org, job, onDone }) {
    const [projectId, setProjectId] = useState(job?.projectId || query().get("project") || "");
    const projects = useApi(job ? null : `/orgs/${org}/overview`);
    const apps = useApi(projectId ? `/projects/${projectId}` : null, [projectId]);
    const [name, setName] = useState(job?.name || "");
    const [schedule, setSchedule] = useState(job?.schedule || "");
    const [concurrency, setConcurrency] = useState(job?.concurrency || "skip");
    const [steps, setSteps] = useState(job?.steps?.map((s) => ({ ...s })) || [{ name: "step 1", type: "script", lang: "bash", source: templates.bash }]);
    useEffect(() => {
        if (!projectId && projects.data?.length)
            setProjectId(projects.data.find((p) => p.canEdit)?.id || "");
    }, [projects.data]);
    const setStep = (i, patch) => setSteps(steps.map((s, j) => (j === i ? { ...s, ...patch } : s)));
    const save = async () => {
        const body = { name, schedule, concurrency, steps, paused: job?.paused || false };
        const res = await act(() => (job ? put(`/jobs/${job.id}`, body) : post(`/projects/${projectId}/jobs`, body)), job ? "Saved" : "Job created");
        if (res) {
            if (onDone)
                onDone();
            else
                navigate(`/o/${org}/jobs/${res.id}`);
        }
    };
    const swallow = steps.some((s) => s.type === "script" && /\|\|\s*true\s*$/m.test(s.source || ""));
    return (h("div", { class: "page-mid" },
        h(Link, { href: job ? `/o/${org}/jobs/${job.id}` : `/o/${org}/jobs`, class: "back-link", onClick: (e) => { if (onDone) {
                e.preventDefault();
                onDone();
            } } },
            h(Icon, { name: "arrow-left", size: 14 }),
            " ",
            job ? job.name : "Jobs"),
        h("div", { class: "page-head" },
            h("h1", { class: "page-title", style: { fontSize: 24 } }, job ? "Edit job" : "New job")),
        h("div", { class: "col gap-lg" },
            h(Card, { class: "form-card" },
                h("div", { class: "form-grid" },
                    h(Field, { label: "Name" },
                        h(Input, { value: name, onInput: (v) => setName(v.toLowerCase().replace(/[^a-z0-9-]/g, "-")), placeholder: "nightly-cleanup", mono: true, autofocus: true })),
                    !job ? (h(Field, { label: "Project" },
                        h(Select, { value: projectId, onChange: setProjectId, options: (projects.data || []).filter((p) => p.canEdit).map((p) => ({ value: p.id, label: `${p.icon} ${p.name}` })) }))) : h(Field, { label: "Project" },
                        h(Input, { value: job.projectName, onInput: () => { }, disabled: true })),
                    h(Field, { label: "Schedule", hint: "Standard cron in UTC, or @hourly / @daily. Empty means it only runs when triggered." },
                        h(Input, { value: schedule, onInput: setSchedule, placeholder: "0 3 * * *", mono: true })),
                    h(Field, { label: "If the previous run is still going" },
                        h(Select, { value: concurrency, onChange: setConcurrency, options: [{ value: "skip", label: "Skip the new run" }, { value: "queue", label: "Queue it" }, { value: "allow", label: "Run both" }] }))),
                h("div", { class: "row row-wrap", style: { gap: 6 } }, [["@hourly", "Hourly"], ["0 3 * * *", "Daily 03:00"], ["*/15 * * * *", "Every 15 min"], ["0 9 * * 1-5", "Weekdays 09:00"], ["", "Manual only"]].map(([v, l]) => (h("button", { key: l, type: "button", class: cx("tag", schedule === v && "active"), style: { cursor: "pointer", borderColor: schedule === v ? "var(--accent)" : undefined }, onClick: () => setSchedule(v) }, l))))),
            steps.map((s, i) => (h(Card, { key: i, class: "form-card" },
                h("div", { class: "row" },
                    h("span", { class: "count-badge" }, i + 1),
                    h(Input, { value: s.name, onInput: (v) => setStep(i, { name: v }), placeholder: "step name" }),
                    h(Segmented, { value: s.type, onChange: (v) => setStep(i, { type: v, source: v === "script" ? s.source || templates.bash : s.source }), options: [{ id: "script", label: "Script", icon: "code" }, { id: "app", label: "App command", icon: "box" }, { id: "image", label: "Custom image", icon: "layers" }] }),
                    h(IconButton, { icon: "chevron-up", title: "Move up", onClick: () => i > 0 && setSteps([...steps.slice(0, i - 1), steps[i], steps[i - 1], ...steps.slice(i + 1)]) }),
                    h(IconButton, { icon: "trash", title: "Remove step", onClick: () => setSteps(steps.filter((_, j) => j !== i)) })),
                s.type === "script" ? (h(Fragment, null,
                    h("div", { class: "row" },
                        h(Segmented, { size: "sm", value: s.lang || "bash", onChange: (v) => setStep(i, { lang: v, source: !s.source || Object.values(templates).includes(s.source) ? templates[v] : s.source }), options: [{ id: "bash", label: "bash" }, { id: "python", label: "python" }, { id: "node", label: "node" }] }),
                        h("span", { class: "muted", style: { fontSize: 12.5 } },
                            "runs in ",
                            h("code", null, { bash: "debian:12-slim", python: "python:3.13-slim", node: "node:22-slim" }[s.lang || "bash"]))),
                    h(Textarea, { value: s.source, onInput: (v) => setStep(i, { source: v }), rows: 8, mono: true }))) : s.type === "app" ? (h("div", { class: "form-grid" },
                    h(Field, { label: "App", hint: "Runs in the app's current image with its variables." },
                        h(Select, { value: s.appId || "", onChange: (v) => setStep(i, { appId: v }), options: [{ value: "", label: "Pick an app…" }, ...((apps.data?.apps || []).map((a) => ({ value: a.id, label: a.name })))] })),
                    h(Field, { label: "Command" },
                        h(Input, { value: s.command || "", onInput: (v) => setStep(i, { command: v }), placeholder: "npm run migrate", mono: true })))) : (h("div", { class: "form-grid" },
                    h(Field, { label: "Image" },
                        h(Input, { value: s.image || "", onInput: (v) => setStep(i, { image: v }), placeholder: "curlimages/curl:latest", mono: true })),
                    h(Field, { label: "Command" },
                        h(Input, { value: s.command || "", onInput: (v) => setStep(i, { command: v }), placeholder: "curl -fsS https://example.com/ping", mono: true }))))))),
            swallow ? h(Callout, { kind: "amber", title: "A step ends in || true." }, "It always reports success, which turns a broken job into a green one. Make sure the job still fails on the thing that actually matters.") : null,
            h(Callout, { kind: "info" },
                "Script images are deliberately small: ",
                h("code", null, "debian:12-slim"),
                " has no ",
                h("code", null, "curl"),
                ". Install what you need on the first line, or use a custom image that has it."),
            h("div", { class: "row" },
                h(Button, { kind: "ghost", icon: "plus", onClick: () => setSteps([...steps, { name: `step ${steps.length + 1}`, type: "script", lang: "bash", source: templates.bash }]) }, "Add step"),
                h("span", { class: "grow" }),
                h(Button, { kind: "primary", icon: "save", onClick: save, disabled: !name || !projectId || !steps.length }, job ? "Save job" : "Create job")))));
}
export function NewJobPage({ org }) {
    return h(JobEditor, { org: org });
}
export function RunPage({ org, id }) {
    const r = useApi(`/runs/${id}`);
    const [open, setOpen] = useState({});
    const [, tick] = useState(0);
    useEvents([`run:${id}`], (e) => {
        if (e.type === "run.updated")
            r.set((d) => ({ ...d, ...e.data }));
    });
    useEffect(() => {
        const t = setInterval(() => tick((n) => n + 1), 1000);
        return () => clearInterval(t);
    }, []);
    if (r.error)
        return h("div", { class: "page-mid" },
            h(ErrorBox, { error: r.error }));
    if (!r.data)
        return h("div", { class: "page-mid" },
            h(Loading, null));
    const run = r.data;
    const live = run.status === "running" || run.status === "queued";
    const isOpen = (i, s) => open[i] ?? (s.status !== "pending" && s.status !== "skipped");
    return (h("div", { class: "page-mid", style: { maxWidth: 944 } },
        h(Link, { href: `/o/${org}/jobs/${run.jobId}`, class: "back-link" },
            h(Icon, { name: "arrow-left", size: 14 }),
            " ",
            run.jobName),
        h("div", { class: "row", style: { gap: 10 } },
            h("h1", { class: "page-title", style: { fontSize: 24 } },
                "Run #",
                run.number),
            h(Pill, { status: runStatus(run.status), label: run.status[0].toUpperCase() + run.status.slice(1) }),
            h("span", { class: "grow" }),
            live ? h(Button, { size: "sm", kind: "secondary", icon: "square", onClick: () => act(() => post(`/runs/${id}/cancel`), "Cancelling") }, "Cancel") : null),
        h("div", { class: "muted", style: { fontSize: 13.5, margin: "4px 0 18px", display: "flex", gap: 14 } },
            h("span", null,
                run.trigger[0].toUpperCase() + run.trigger.slice(1),
                run.createdBy ? " · " + run.createdBy : ""),
            h("span", null,
                "started ",
                timeAgo(run.startedAt || run.createdAt)),
            h("span", null, duration(run.startedAt || run.createdAt, run.finishedAt))),
        h(Card, { class: "run-card" },
            h("div", { class: "col", style: { gap: 18 } }, run.steps.map((s, i) => (h("div", { key: i },
                h("span", { class: "run-tab" },
                    h(Icon, { name: "file", size: 11 }),
                    " ",
                    s.type === "app" ? "app" : s.type === "image" ? "image" : "script"),
                h("div", { class: "run-step" },
                    h("div", { class: "run-step-head", onClick: () => setOpen({ ...open, [i]: !isOpen(i, s) }) },
                        h("span", { class: cx("step-check", s.status) }, s.status === "succeeded" ? h(Icon, { name: "check", size: 12, stroke: 2.5 }) : s.status === "failed" ? h(Icon, { name: "x", size: 12, stroke: 2.5 }) : null),
                        h("span", { class: "run-step-name" }, s.name),
                        h("span", { class: "run-step-image" }, s.image),
                        h("span", { class: "run-step-dur" }, s.startedAt ? duration(s.startedAt, s.finishedAt) : s.status))),
                isOpen(i, s) ? (h(Fragment, null,
                    h("div", { class: "run-out" },
                        h(OutputView, { text: s.output, onRefresh: r.reload, height: 420 })),
                    s.exitCode != null ? h("div", { class: "run-exit" },
                        "exit code ",
                        s.exitCode) : null)) : null)))))));
}
export { get, Toggle, plural };
