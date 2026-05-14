const externalProtocols = new Set(["http:", "https:", "mailto:"]);

export function isTrustedNavigationURL(rawURL: string, rendererBaseURL: string | undefined): boolean {
  return isTrustedRendererURL(rawURL, rendererBaseURL);
}

export function isTrustedRendererURL(rawURL: string, rendererBaseURL: string | undefined): boolean {
  const url = parseURL(rawURL);
  const rendererURL = parseURL(rendererBaseURL);
  if (!url || !rendererURL) return false;
  return url.origin === rendererURL.origin;
}

export function isSafeExternalURL(rawURL: string): boolean {
  const url = parseURL(rawURL);
  return Boolean(url && externalProtocols.has(url.protocol));
}

function parseURL(rawURL: string | undefined): URL | null {
  if (!rawURL) return null;
  try {
    return new URL(rawURL);
  } catch {
    return null;
  }
}
