/// <reference types="vite/client" />

import type { YardDesktopBridge } from "@/platform";

declare global {
  interface Window {
    yardDesktop?: YardDesktopBridge;
  }
}
