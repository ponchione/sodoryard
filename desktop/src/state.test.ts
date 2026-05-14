import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  normalizeDesktopState,
  readDesktopState,
  updateDesktopState,
  writeDesktopState,
} from "./state.js";

test("readDesktopState tolerates missing and malformed state", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "yard-desktop-state-"));
  assert.deepEqual(readDesktopState(dir), {});

  fs.writeFileSync(path.join(dir, "desktop-state.json"), "{broken", "utf8");
  assert.deepEqual(readDesktopState(dir), {});
});

test("writeDesktopState persists normalized recent project and window bounds", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "yard-desktop-state-"));
  writeDesktopState(dir, {
    recentProjectRoot: "/tmp/project",
    window: { x: 12.9, y: 34.1, width: 100, height: 200, maximized: true },
  });

  assert.deepEqual(readDesktopState(dir), {
    recentProjectRoot: "/tmp/project",
    window: { x: 12, y: 34, width: 980, height: 680, maximized: true },
  });
});

test("updateDesktopState merges patches over existing state", () => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "yard-desktop-state-"));
  writeDesktopState(dir, { recentProjectRoot: "/tmp/old", window: { width: 1200, height: 800 } });

  const next = updateDesktopState(dir, { recentProjectRoot: "/tmp/new" });

  assert.deepEqual(next, {
    recentProjectRoot: "/tmp/new",
    window: { width: 1200, height: 800 },
  });
});

test("normalizeDesktopState drops invalid top-level fields", () => {
  assert.deepEqual(normalizeDesktopState({
    recentProjectRoot: "",
    window: "wide",
  }), {});
});
