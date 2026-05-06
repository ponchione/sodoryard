export type ChainStatusGroup = "active" | "success" | "failed" | "other";

export function chainStatusGroup(status: string): ChainStatusGroup {
  if (status === "running" || status === "paused" || status.endsWith("_requested")) return "active";
  if (status === "completed" || status === "dry_run") return "success";
  if (status === "failed" || status === "cancelled") return "failed";
  return "other";
}

export function chainStatusClass(status: string): string {
  switch (chainStatusGroup(status)) {
  case "active":
    return "text-warning";
  case "success":
    return "text-accent";
  case "failed":
    return "text-destructive";
  default:
    return "text-muted-foreground";
  }
}
