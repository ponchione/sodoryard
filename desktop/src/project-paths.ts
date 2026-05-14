import path from "node:path";

export function toProjectRelativeFilePaths(projectRoot: string | undefined, filePaths: string[]): string[] {
  const root = projectRoot?.trim();
  if (!root) return [];
  const absRoot = path.resolve(root);
  const seen = new Set<string>();
  const out: string[] = [];
  for (const filePath of filePaths) {
    const relPath = projectRelativePath(absRoot, filePath);
    if (!relPath || seen.has(relPath)) continue;
    seen.add(relPath);
    out.push(relPath);
  }
  return out;
}

function projectRelativePath(absRoot: string, filePath: string): string | null {
  const rawPath = filePath.trim();
  if (!rawPath) return null;
  const relPath = path.relative(absRoot, path.resolve(rawPath));
  if (!relPath || relPath === ".." || relPath.startsWith(`..${path.sep}`) || path.isAbsolute(relPath)) {
    return null;
  }
  return relPath.replaceAll(path.sep, "/");
}
