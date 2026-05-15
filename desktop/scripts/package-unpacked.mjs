import { createRequire } from "node:module";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const require = createRequire(import.meta.url);
const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const desktopRoot = path.resolve(scriptDir, "..");
const repoRoot = path.resolve(desktopRoot, "..");
const outRoot = path.join(desktopRoot, "out");
const packageName = `Yard-${process.platform}-${process.arch}`;
const packageDir = path.join(outRoot, packageName);

const yardBinary = path.join(repoRoot, "bin", process.platform === "win32" ? "yard.exe" : "yard");
const webDist = path.join(repoRoot, "web", "dist");
const desktopDist = path.join(desktopRoot, "dist");
const desktopAssets = path.join(desktopRoot, "assets");
const desktopIcon = path.join(desktopAssets, "yard-desktop.svg");
const lanceDBLibDir = path.join(repoRoot, "lib", "linux_amd64");
const electronPackage = JSON.parse(fs.readFileSync(path.join(desktopRoot, "node_modules", "electron", "package.json"), "utf8"));

validateInputs();

const electronPath = require("electron");
const electronDist = path.dirname(electronPath);

fs.rmSync(packageDir, { recursive: true, force: true });
fs.mkdirSync(packageDir, { recursive: true });
fs.cpSync(electronDist, packageDir, { recursive: true });

const appDir = path.join(packageDir, "resources", "app");
fs.rmSync(appDir, { recursive: true, force: true });
fs.mkdirSync(appDir, { recursive: true });
fs.cpSync(desktopDist, path.join(appDir, "dist"), { recursive: true });
fs.cpSync(desktopAssets, path.join(appDir, "assets"), { recursive: true });
fs.writeFileSync(path.join(appDir, "package.json"), packageJSON(), "utf8");
fs.writeFileSync(path.join(appDir, "yard-source.json"), sourceMetadata(), "utf8");

fs.mkdirSync(path.join(packageDir, "resources", "yard"), { recursive: true });
fs.copyFileSync(yardBinary, path.join(packageDir, "resources", "yard", path.basename(yardBinary)));
if (process.platform !== "win32") {
  fs.chmodSync(path.join(packageDir, "resources", "yard", path.basename(yardBinary)), 0o755);
}

if (fs.existsSync(lanceDBLibDir)) {
  fs.cpSync(lanceDBLibDir, path.join(packageDir, "resources", "lib"), { recursive: true });
}
fs.cpSync(webDist, path.join(packageDir, "resources", "web-dist"), { recursive: true });

writeLauncher(packageDir);
writeDesktopEntry(packageDir);
writeManifest(packageDir);

console.log(`Packaged Yard Desktop to ${path.relative(repoRoot, packageDir)}`);

function validateInputs() {
  assertExists(yardBinary, "yard sidecar binary");
  assertExists(webDist, "web production build");
  assertExists(desktopDist, "desktop main/preload build");
  assertExists(desktopIcon, "desktop icon");
}

function assertExists(target, label) {
  if (!fs.existsSync(target)) {
    throw new Error(`Missing ${label}: ${target}`);
  }
}

function packageJSON() {
  const source = JSON.parse(fs.readFileSync(path.join(desktopRoot, "package.json"), "utf8"));
  return `${JSON.stringify({
    name: source.name,
    productName: source.productName,
    version: source.version,
    type: "module",
    desktopName: source.desktopName,
    main: "dist/main.js",
  }, null, 2)}\n`;
}

function sourceMetadata() {
  return `${JSON.stringify({
    sourceRoot: repoRoot,
    packagedAt: new Date().toISOString(),
  }, null, 2)}\n`;
}

function writeLauncher(targetDir) {
  if (process.platform !== "linux") return;
  const launcherPath = path.join(targetDir, "yard-desktop");
  fs.writeFileSync(launcherPath, `#!/usr/bin/env bash
set -euo pipefail
APP_DIR="$(cd "$(dirname "\${BASH_SOURCE[0]}")" && pwd)"
export YARD_DESKTOP_PACKAGED=1
export YARD_BINARY="$APP_DIR/resources/yard/yard"
export YARD_SOURCE_ROOT=${shellQuote(repoRoot)}
export LD_LIBRARY_PATH="$APP_DIR/resources/lib:\${LD_LIBRARY_PATH:-}"
exec "$APP_DIR/electron" --no-sandbox --class=yard-desktop "$APP_DIR/resources/app" "$@"
`, "utf8");
  fs.chmodSync(launcherPath, 0o755);
}

function writeDesktopEntry(targetDir) {
  if (process.platform !== "linux") return;
  const launcherPath = path.join(targetDir, "yard-desktop");
  const iconPath = path.join(targetDir, "resources", "app", "assets", "yard-desktop.svg");
  const desktopEntryPath = path.join(targetDir, "yard-desktop.desktop");
  fs.writeFileSync(desktopEntryPath, desktopEntry({
    exec: launcherPath,
    icon: iconPath,
  }), "utf8");
}

function writeManifest(targetDir) {
  const manifest = {
    app: "Yard Desktop",
    package: packageName,
    platform: process.platform,
    arch: process.arch,
    createdAt: new Date().toISOString(),
    node: process.version,
    electron: electronPackage.version,
    os: `${os.type()} ${os.release()}`,
  };
  fs.writeFileSync(path.join(targetDir, "yard-desktop-package.json"), `${JSON.stringify(manifest, null, 2)}\n`, "utf8");
}

function desktopEntry({ exec, icon }) {
  return `[Desktop Entry]
Type=Application
Name=Yard Desktop
Comment=Local Yard operator console
Exec=${quoteExecPath(exec)} %U
Icon=${icon}
Terminal=false
Categories=Development;
StartupNotify=true
StartupWMClass=yard-desktop
`;
}

function quoteExecPath(value) {
  return `"${String(value).replace(/(["\\`$])/g, "\\$1")}"`;
}

function shellQuote(value) {
  return `'${String(value).replace(/'/g, `'\"'\"'`)}'`;
}
