import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { resolveBackendProjectDir, resolveDefaultProjectDir } from "./backend.js";

test("resolveDefaultProjectDir uses packaged source metadata before Electron app path fallback", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "yard-desktop-backend-"));
  const appPath = path.join(dir, "desktop", "out", "Yard-linux-x64", "resources", "app");
  const sourceRoot = path.join(dir, "source", "sodoryard");
  fs.mkdirSync(appPath, { recursive: true });
  fs.mkdirSync(sourceRoot, { recursive: true });
  fs.writeFileSync(path.join(appPath, "package.json"), "{}\n", "utf8");
  fs.writeFileSync(path.join(appPath, "yard-source.json"), JSON.stringify({ sourceRoot }), "utf8");

  assert.equal(resolveDefaultProjectDir(appPath), sourceRoot);
});

test("resolveDefaultProjectDir lets YARD_SOURCE_ROOT override packaged metadata", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "yard-desktop-backend-"));
  const appPath = path.join(dir, "resources", "app");
  const metadataRoot = path.join(dir, "metadata-root");
  const envRoot = path.join(dir, "env-root");
  fs.mkdirSync(appPath, { recursive: true });
  fs.writeFileSync(path.join(appPath, "package.json"), "{}\n", "utf8");
  fs.writeFileSync(path.join(appPath, "yard-source.json"), JSON.stringify({ sourceRoot: metadataRoot }), "utf8");
  const original = process.env.YARD_SOURCE_ROOT;
  process.env.YARD_SOURCE_ROOT = envRoot;
  try {
    assert.equal(resolveDefaultProjectDir(appPath), envRoot);
  } finally {
    if (original === undefined) {
      delete process.env.YARD_SOURCE_ROOT;
    } else {
      process.env.YARD_SOURCE_ROOT = original;
    }
  }
});

test("resolveBackendProjectDir ignores stale package resources project state", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "yard-desktop-backend-"));
  const appPath = path.join(dir, "desktop", "out", "Yard-linux-x64", "resources", "app");
  const resourcesPath = path.dirname(appPath);
  const sourceRoot = path.join(dir, "source", "sodoryard");
  fs.mkdirSync(appPath, { recursive: true });
  fs.writeFileSync(path.join(appPath, "package.json"), "{}\n", "utf8");
  fs.writeFileSync(path.join(appPath, "yard-source.json"), JSON.stringify({ sourceRoot }), "utf8");

  assert.equal(resolveBackendProjectDir(appPath, resourcesPath), sourceRoot);
  assert.equal(resolveBackendProjectDir(appPath, path.join(resourcesPath, "app")), sourceRoot);
});

test("resolveBackendProjectDir keeps a valid requested project outside packaged resources", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "yard-desktop-backend-"));
  const appPath = path.join(dir, "desktop", "out", "Yard-linux-x64", "resources", "app");
  const requestedProject = path.join(dir, "workspace");
  fs.mkdirSync(appPath, { recursive: true });

  assert.equal(resolveBackendProjectDir(appPath, requestedProject), requestedProject);
});
