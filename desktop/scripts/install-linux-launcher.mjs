import childProcess from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

if (process.platform !== "linux") {
  throw new Error("Yard Desktop user launcher installation is currently implemented for Linux only.");
}

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const desktopRoot = path.resolve(scriptDir, "..");
const packageDir = process.env.YARD_DESKTOP_PACKAGE_DIR
  ? path.resolve(process.env.YARD_DESKTOP_PACKAGE_DIR)
  : path.join(desktopRoot, "out", `Yard-${process.platform}-${process.arch}`);
const launcherPath = path.join(packageDir, "yard-desktop");
const packageIconPath = path.join(packageDir, "resources", "app", "assets", "yard-desktop.svg");
const sourceIconPath = fs.existsSync(packageIconPath)
  ? packageIconPath
  : path.join(desktopRoot, "assets", "yard-desktop.svg");

assertExists(launcherPath, "packaged Yard Desktop launcher");
assertExists(sourceIconPath, "Yard Desktop icon");

const dataHome = process.env.XDG_DATA_HOME || path.join(os.homedir(), ".local", "share");
const applicationsDir = path.join(dataHome, "applications");
const iconsDir = path.join(dataHome, "icons", "hicolor", "scalable", "apps");
const desktopEntryPath = path.join(applicationsDir, "yard-desktop.desktop");
const installedIconPath = path.join(iconsDir, "yard-desktop.svg");

fs.mkdirSync(applicationsDir, { recursive: true });
fs.mkdirSync(iconsDir, { recursive: true });
fs.copyFileSync(sourceIconPath, installedIconPath);
fs.writeFileSync(desktopEntryPath, desktopEntry(launcherPath), "utf8");
fs.chmodSync(desktopEntryPath, 0o644);

refreshDesktopCaches(applicationsDir);

console.log(`Installed Yard Desktop launcher: ${desktopEntryPath}`);
console.log(`Installed Yard Desktop icon: ${installedIconPath}`);
console.log("Open your application menu for 'Yard Desktop', then pin it to your taskbar or dock.");

function assertExists(target, label) {
  if (!fs.existsSync(target)) {
    throw new Error(`Missing ${label}: ${target}. Run 'make desktop-package' first.`);
  }
}

function desktopEntry(execPath) {
  return `[Desktop Entry]
Type=Application
Name=Yard Desktop
Comment=Local Yard operator console
Exec=${quoteExecPath(execPath)} %U
Icon=yard-desktop
Terminal=false
Categories=Development;
StartupNotify=true
StartupWMClass=yard-desktop
`;
}

function quoteExecPath(value) {
  return `"${String(value).replace(/(["\\`$])/g, "\\$1")}"`;
}

function refreshDesktopCaches(applicationsDir) {
  runOptional("update-desktop-database", [applicationsDir]);
  runOptional("xdg-desktop-menu", ["forceupdate"]);
  runOptional("xdg-icon-resource", ["forceupdate"]);
}

function runOptional(command, args) {
  childProcess.spawnSync(command, args, { stdio: "ignore" });
}
