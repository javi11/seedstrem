import { ComponentType, FormEvent, useEffect, useMemo, useRef, useState } from "react";
import { api, Config } from "../../api";
import { useToast } from "../../components/Toast";
import { useNavigationGuard } from "../../components/NavigationGuard";
import { PageHeader } from "../../components/PageHeader";
import { Skeleton } from "../../components/Skeleton";
import { SectionDef, SectionProps } from "./types";
import { DownloadClient } from "./sections/DownloadClient";
import { Prowlarr } from "./sections/Prowlarr";
import { ContentTypes } from "./sections/ContentTypes";
import { Filters } from "./sections/Filters";
import { Metadata } from "./sections/Metadata";
import { Seeding } from "./sections/Seeding";
import { Rss } from "./sections/Rss";
import { PathMappings } from "./sections/PathMappings";
import { Server } from "./sections/Server";
import { Streaming } from "./sections/Streaming";

const SECTIONS: SectionDef[] = [
  { id: "download-client", label: "Download client", icon: "⇩", group: "Connections" },
  { id: "prowlarr", label: "Prowlarr", icon: "🔎", group: "Connections" },
  { id: "content-types", label: "Content types", icon: "▶", group: "Addon" },
  { id: "filters", label: "Result filters", icon: "⛃", group: "Addon" },
  { id: "metadata", label: "Metadata", icon: "🎬", group: "Addon", restart: true },
  { id: "seeding", label: "Seeding & cleanup", icon: "♺", group: "System" },
  { id: "rss", label: "RSS auto-grab", icon: "📡", group: "System" },
  { id: "paths", label: "Path mappings", icon: "🗺", group: "System" },
  { id: "server", label: "Server", icon: "🖥", group: "System", restart: true },
  { id: "streaming", label: "Streaming", icon: "⇄", group: "System" },
];

const COMPONENTS: Record<string, ComponentType<SectionProps>> = {
  "download-client": DownloadClient,
  prowlarr: Prowlarr,
  "content-types": ContentTypes,
  filters: Filters,
  metadata: Metadata,
  seeding: Seeding,
  rss: Rss,
  paths: PathMappings,
  server: Server,
  streaming: Streaming,
};

const GROUP_ORDER = ["Connections", "Addon", "System"];

// Secrets arrive masked; blank them so the "empty = keep existing" convention
// holds and the mask is never resubmitted.
function blankSecrets(c: Config): Config {
  c.qbittorrent.password = "";
  c.deluge.password = "";
  c.server.admin_password = "";
  c.prowlarr.api_key = "";
  c.meta.tmdb_api_key = "";
  // Configs predating indexer_ids arrive without it (JSON null).
  c.prowlarr.indexer_ids = c.prowlarr.indexer_ids ?? [];
  return c;
}

export function Settings() {
  const toast = useToast();
  const { setGuard } = useNavigationGuard();

  const [config, setConfig] = useState<Config | null>(null);
  const [saved, setSaved] = useState<string>(""); // JSON snapshot of last-saved state
  const [active, setActive] = useState(SECTIONS[0].id);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    api
      .getConfig()
      .then((c) => {
        const blanked = blankSecrets(c);
        setConfig(blanked);
        setSaved(JSON.stringify(blanked));
      })
      .catch(() => toast.error("Couldn't load configuration"));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const dirty = config !== null && JSON.stringify(config) !== saved;

  // Register the unsaved-changes guard for in-app navigation, plus a native
  // beforeunload prompt for reload/close.
  const dirtyRef = useRef(false);
  dirtyRef.current = dirty;
  useEffect(() => {
    setGuard(() => dirtyRef.current);
    return () => setGuard(null);
  }, [setGuard]);
  useEffect(() => {
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      if (dirtyRef.current) {
        e.preventDefault();
        e.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => window.removeEventListener("beforeunload", onBeforeUnload);
  }, []);

  const update = (fn: (c: Config) => void) =>
    setConfig((prev) => {
      if (!prev) return prev;
      const next = structuredClone(prev);
      fn(next);
      return next;
    });

  async function save(e: FormEvent) {
    e.preventDefault();
    if (!config) return;
    setSaving(true);
    try {
      const res = await api.putConfig(config);
      const blanked = blankSecrets(res.config);
      setConfig(blanked);
      setSaved(JSON.stringify(blanked));
      if (res.restart_required) {
        toast.info("Saved — some changes apply only after restarting seedstrem");
      } else {
        toast.success("Settings saved");
      }
    } catch (err) {
      toast.error(`Couldn't save — ${(err as Error).message}`);
    } finally {
      setSaving(false);
    }
  }

  const grouped = useMemo(() => {
    return GROUP_ORDER.map((group) => ({
      group,
      items: SECTIONS.filter((s) => s.group === group),
    }));
  }, []);

  if (!config) {
    return (
      <div className="flex flex-col gap-6">
        <PageHeader title="Settings" />
        <Skeleton className="h-96 w-full" />
      </div>
    );
  }

  const ActiveSection = COMPONENTS[active];
  const activeDef = SECTIONS.find((s) => s.id === active)!;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title="Settings" />

      {/* Mobile section selector */}
      <select
        className="select select-bordered md:hidden"
        value={active}
        onChange={(e) => setActive(e.target.value)}
      >
        {SECTIONS.map((s) => (
          <option key={s.id} value={s.id}>
            {s.label}
            {s.restart ? " (restart)" : ""}
          </option>
        ))}
      </select>

      <form onSubmit={save} className="flex gap-6">
        {/* Desktop sub-nav */}
        <nav className="surface hidden w-56 shrink-0 self-start p-2 md:block">
          {grouped.map(({ group, items }) => (
            <div key={group} className="mb-2">
              <div className="px-3 py-1.5 text-[10px] font-semibold uppercase tracking-wider opacity-50">
                {group}
              </div>
              {items.map((s) => (
                <button
                  key={s.id}
                  type="button"
                  onClick={() => setActive(s.id)}
                  className={[
                    "flex w-full items-center gap-2 rounded-field px-3 py-2 text-left text-sm transition-colors",
                    active === s.id
                      ? "bg-base-300 font-medium text-base-content shadow-[inset_2px_0_0_0_var(--color-primary)]"
                      : "text-base-content/70 hover:bg-base-300/60",
                  ].join(" ")}
                >
                  <span className="w-4 text-center" aria-hidden>
                    {s.icon}
                  </span>
                  <span className="flex-1 truncate">{s.label}</span>
                  {s.restart && (
                    <span className="badge badge-warning badge-xs" title="Restart required">
                      ↻
                    </span>
                  )}
                </button>
              ))}
            </div>
          ))}
        </nav>

        {/* Active section + sticky save bar */}
        <div className="min-w-0 flex-1">
          {activeDef.restart && (
            <div className="alert alert-warning mb-4 py-2 text-sm">
              <span>Changes in this section apply only after restarting seedstrem.</span>
            </div>
          )}
          <ActiveSection config={config} update={update} />

          <div className="sticky bottom-0 z-10 mt-4 flex items-center justify-between gap-3 rounded-box border border-base-content/10 bg-base-100/95 p-3 backdrop-blur">
            <span className="text-sm">
              {dirty ? (
                <span className="flex items-center gap-2 text-warning">
                  <span className="inline-block h-2 w-2 rounded-full bg-warning" />
                  Unsaved changes
                </span>
              ) : (
                <span className="opacity-50">All changes saved</span>
              )}
            </span>
            <button className="btn btn-primary" disabled={saving || !dirty}>
              {saving ? <span className="loading loading-spinner loading-sm" /> : "Save settings"}
            </button>
          </div>
        </div>
      </form>
    </div>
  );
}
