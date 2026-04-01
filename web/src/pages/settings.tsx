import { useEffect, useState } from "react";
import { useProviders } from "@/hooks/use-providers";
import { useProjectInfo } from "@/hooks/use-project-info";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import type { AppConfig } from "@/types/metrics";

export function SettingsPage() {
  const { providers, loading: provLoading } = useProviders();
  const { project, loading: projLoading } = useProjectInfo();
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [configLoading, setConfigLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [saveMsg, setSaveMsg] = useState<string | null>(null);

  // Load config on mount.
  useEffect(() => {
    api
      .get<AppConfig>("/api/config")
      .then((c) => {
        setConfig(c);
        setConfigLoading(false);
      })
      .catch(() => setConfigLoading(false));
  }, []);

  const handleModelChange = async (provider: string, model: string) => {
    try {
      setSaving(true);
      setSaveMsg(null);
      const updated = await api.put<AppConfig>("/api/config", {
        default_provider: provider,
        default_model: model,
      });
      setConfig(updated);
      setSaveMsg("Saved");
      setTimeout(() => setSaveMsg(null), 2000);
    } catch (err) {
      setSaveMsg(err instanceof Error ? err.message : "Failed to save");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex-1 overflow-y-auto px-4 py-6">
      <div className="mx-auto max-w-2xl space-y-6">
        <h1 className="text-xl font-bold">Settings</h1>

        {/* Project Info */}
        <section className="space-y-2">
          <h2 className="text-sm font-semibold text-muted-foreground">Project</h2>
          {projLoading ? (
            <p className="text-xs text-muted-foreground">Loading…</p>
          ) : project ? (
            <div className="rounded-lg border border-border p-3 text-sm space-y-1">
              <div className="flex justify-between">
                <span className="text-muted-foreground">Name</span>
                <span className="font-medium">{project.name}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Path</span>
                <span className="font-mono text-xs">{project.root_path}</span>
              </div>
              {project.language && (
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Language</span>
                  <span>{project.language}</span>
                </div>
              )}
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">No project info available</p>
          )}
        </section>

        {/* Default Model */}
        <section className="space-y-2">
          <h2 className="text-sm font-semibold text-muted-foreground">Default Model</h2>
          {configLoading ? (
            <p className="text-xs text-muted-foreground">Loading…</p>
          ) : config ? (
            <div className="rounded-lg border border-border p-3 space-y-3">
              <div className="flex items-center gap-2 text-sm">
                <span className="text-muted-foreground">Current:</span>
                <span className="font-mono font-medium">
                  {config.default_provider}/{config.default_model}
                </span>
              </div>
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                <span>Fallback:</span>
                <span className="font-mono">
                  {config.fallback_provider}/{config.fallback_model}
                </span>
              </div>

              {/* Model selector */}
              {providers.length > 0 && (
                <div className="space-y-1">
                  {providers.map((prov) => (
                    <div key={prov.name}>
                      {prov.models.map((model) => {
                        const isActive =
                          config.default_provider === prov.name &&
                          config.default_model === model.id;
                        return (
                          <Button
                            key={model.id}
                            variant={isActive ? "default" : "ghost"}
                            size="sm"
                            className="mr-1 mb-1 text-xs"
                            disabled={saving || isActive}
                            onClick={() => handleModelChange(prov.name, model.id)}
                          >
                            {prov.name}/{model.id}
                          </Button>
                        );
                      })}
                    </div>
                  ))}
                </div>
              )}

              {saveMsg && (
                <p className={`text-xs ${saveMsg === "Saved" ? "text-green-600" : "text-destructive"}`}>
                  {saveMsg}
                </p>
              )}

              <div className="text-[10px] text-muted-foreground/60">
                Agent: max {config.agent.max_iterations} iterations, extended thinking{" "}
                {config.agent.extended_thinking ? "on" : "off"}
              </div>
            </div>
          ) : null}
        </section>

        {/* Providers */}
        <section className="space-y-2">
          <h2 className="text-sm font-semibold text-muted-foreground">Providers</h2>
          {provLoading ? (
            <p className="text-xs text-muted-foreground">Loading…</p>
          ) : providers.length === 0 ? (
            <p className="text-xs text-muted-foreground">No providers configured</p>
          ) : (
            <div className="space-y-2">
              {providers.map((prov) => (
                <div key={prov.name} className="rounded-lg border border-border p-3">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2">
                      <span className="text-sm font-medium">{prov.name}</span>
                      <span className="rounded bg-muted px-1.5 py-0.5 text-[10px] text-muted-foreground">
                        {prov.type}
                      </span>
                    </div>
                    <span
                      className={`rounded-full px-2 py-0.5 text-[10px] font-medium ${
                        prov.status === "available"
                          ? "bg-green-500/20 text-green-700 dark:text-green-300"
                          : "bg-red-500/20 text-red-700 dark:text-red-300"
                      }`}
                    >
                      {prov.status}
                    </span>
                  </div>
                  {prov.models.length > 0 && (
                    <div className="mt-2 space-y-0.5">
                      {prov.models.map((m) => (
                        <div key={m.id} className="flex items-center gap-2 text-xs text-muted-foreground">
                          <span className="font-mono">{m.id}</span>
                          <span className="text-[10px]">{(m.context_window / 1000).toFixed(0)}k ctx</span>
                          {m.supports_tools && <span className="text-[10px]">🔧</span>}
                          {m.supports_thinking && <span className="text-[10px]">💭</span>}
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </section>
      </div>
    </div>
  );
}
