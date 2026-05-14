export type BackendLaunchMode = "managed" | "attached";

export interface DesktopProjectMemoryInfo {
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

export interface DesktopPlatformInfo {
  kind: "desktop";
  appVersion: string;
  yardVersion?: string;
  apiVersion?: string;
  backendBaseUrl: string;
  backendDirectUrl: string;
  backendLaunchMode: BackendLaunchMode;
  projectRoot?: string;
  configPath?: string;
  recentProjectRoot?: string;
  capabilities: string[];
  projectMemory?: DesktopProjectMemoryInfo;
}

export interface DesktopCapabilitiesResponse {
  yard_version?: string;
  api_version?: string;
  project_root?: string;
  config_path?: string;
  capabilities?: string[];
  project_memory?: {
    backend?: string;
    module?: string;
    schema_version?: number;
    contract_version?: number;
    shunter_version?: string;
    default_subprotocol?: string;
    supported_subprotocols?: string[];
    subscribe_url?: string;
    generated_binding_hash?: string;
  };
}

export interface ProjectMemoryTokenResponse {
  token?: string;
  token_type?: string;
  identity?: string;
  expires_at?: string;
}

export interface YardNotification {
  title: string;
  body?: string;
}
