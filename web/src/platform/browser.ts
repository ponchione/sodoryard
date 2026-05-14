import type { YardPlatform } from "./types";

export function createBrowserPlatform(): YardPlatform {
  return {
    kind: "browser",
    backendBaseUrl: "",
    async openExternal(url) {
      window.open(url, "_blank", "noopener,noreferrer");
    },
  };
}
