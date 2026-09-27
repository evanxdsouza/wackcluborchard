import { h, useEffect, useState } from "../lib/sprout.js";
import { Link } from "../lib/router.js";
import { useApi, useEvents } from "../lib/api.js";
import { duration, timeAgo, shortSha, appURL } from "../lib/format.js";
import { Loading, ErrorBox, Pill, Callout, cx, Tag } from "../ui/kit.js";
import { Icon } from "../ui/icons.js";
import { DeployLog } from "../ui/term.js";
export function DeployPage({ org, id }) {
    const r = useApi(`/deploys/${id}`);
    const app = useApi(r.data ? `/apps/${r.data.appId}` : null, [r.data?.appId]);
    const [, tick] = useState(0);
    useEvents([`deploy:${id}`], (e) => {
        if (e.type === "deploy.updated")
            r.set(() => e.data);
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
    const d = r.data;
    const live = d.status === "running" || d.status === "queued";
    return (h("div", { class: "page-mid" },
        h(Link, { href: `/o/${org}/apps/${d.appId}?tab=deploys`, class: "back-link" },
            h(Icon, { name: "arrow-left", size: 14 }),
            " ",
            app.data?.name || "app"),
        h("div", { class: "page-head", style: { marginBottom: 10 } },
            h("h1", { class: "page-title", style: { fontSize: 24 } },
                "Deploy #",
                d.number),
            h(Pill, { status: d.status === "running" ? "deploying" : d.status }),
            h("span", { class: "muted", style: { fontSize: 13.5 } },
                d.kind === "rollback" ? d.message : d.kind === "build" ? "Build from source" : "Image deploy",
                " \u00B7 ",
                d.trigger,
                " \u00B7 ",
                d.createdBy,
                " \u00B7 started ",
                timeAgo(d.createdAt),
                " \u00B7 ",
                duration(d.createdAt, d.finishedAt))),
        h("div", { class: "row row-wrap", style: { marginBottom: 16 } },
            d.commit ? h(Tag, { mono: true },
                h(Icon, { name: "commit", size: 12 }),
                " ",
                shortSha(d.commit)) : null,
            d.message && d.kind !== "rollback" ? h(Tag, null, d.message) : null,
            d.image ? h(Tag, { mono: true }, d.image) : null),
        h("div", { class: "step-bar" }, d.steps.map((s) => h("div", { key: s.name, class: cx("step-seg", s.status) }))),
        h("div", { class: "grid-2", style: { gridTemplateColumns: "minmax(0, 300px) minmax(0, 1fr)", alignItems: "start" } },
            h("div", { class: "steps" }, d.steps.map((s, i) => (h("div", { key: s.name, class: cx("step", s.status) },
                h("div", { class: "step-icon" }, s.status === "succeeded" ? h(Icon, { name: "check", size: 13, stroke: 2.5 }) : s.status === "failed" ? h(Icon, { name: "x", size: 13, stroke: 2.5 }) : s.status === "running" ? h("span", { class: "spinner sm" }) : h("span", { style: { fontSize: 11, fontWeight: 600 } }, i + 1)),
                h("span", { class: "step-name" }, s.name),
                h("span", { class: "step-time" }, s.startedAt ? duration(s.startedAt, s.finishedAt) : ""))))),
            h("div", { class: "col" },
                d.status === "failed" ? (h(Callout, { kind: "red", title: "The deploy failed." },
                    d.error,
                    " Nothing was torn down: the previous version keeps running. Fix the problem and redeploy; there is nothing to recreate.")) : d.status === "succeeded" ? (h(Callout, { kind: "green", title: "Live." },
                    "Rolled out and passing health checks. ",
                    app.data?.domains?.[0] ? h("a", { href: appURL(app.data.domains[0].host), target: "_blank", rel: "noopener" },
                        "Open ",
                        app.data.domains[0].host,
                        " \u2197") : null)) : d.status === "queued" ? (h(Callout, { kind: "info", title: "Queued." }, "All build slots are busy. This build starts on its own when one frees up; there is no need to retry.")) : d.status === "superseded" ? (h(Callout, { kind: "info" }, "A newer deploy replaced this one before it started.")) : null,
                d.kind === "build" || live || d.status === "failed" ? h(DeployLog, { deployId: d.id, height: 460 }) : null))));
}
