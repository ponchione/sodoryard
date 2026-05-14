export interface YardNotification {
  title: string;
  body?: string;
}

export interface YardProjectMemoryPlatform {
  backend?: string;
  module?: string;
  schemaVersion?: number;
  contractVersion?: number;
  shunterVersion?: string;
  defaultSubprotocol?: string;
  supportedSubprotocols?: string[];
  subscribeUrl?: string;
  token?: string;
  generatedBindingHash?: string;
}

export interface YardDesktopPlatformInfo {
  kind: "desktop";
  appVersion: string;
  yardVersion?: string;
  apiVersion?: string;
  backendBaseUrl: string;
  backendDirectUrl?: string;
  backendLaunchMode?: "managed" | "attached";
  desktopSessionToken?: string;
  projectRoot?: string;
  configPath?: string;
  recentProjectRoot?: string;
  capabilities: string[];
  projectMemory?: YardProjectMemoryPlatform;
}

export interface YardPlatform {
  kind: "browser" | "desktop";
  backendBaseUrl: string;
  desktopSessionToken?: string;
  projectMemory?: YardProjectMemoryPlatform;
  openExternal(url: string): Promise<void>;
  chooseProjectDirectory?(): Promise<string | null>;
  chooseProjectFiles?(): Promise<string[]>;
  openProjectPath?(path: string): Promise<void>;
  revealProjectPath?(path: string): Promise<void>;
  notify?(notification: YardNotification): Promise<void>;
  getAppInfo?(): Promise<YardDesktopPlatformInfo | null>;
}

export interface YardDesktopBridge {
  getPlatformInfo(): YardDesktopPlatformInfo | null;
  openExternal(url: string): Promise<void>;
  chooseProjectDirectory?(): Promise<string | null>;
  chooseProjectFiles?(): Promise<string[]>;
  openProjectPath?(path: string): Promise<void>;
  revealProjectPath?(path: string): Promise<void>;
  notify?(notification: YardNotification): Promise<void>;
}
