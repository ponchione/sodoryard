import assert from "node:assert/strict";
import path from "node:path";
import test from "node:test";

import { toProjectAbsolutePath, toProjectRelativeFilePaths } from "./project-paths.js";

test("toProjectRelativeFilePaths returns unique project-relative paths", () => {
  const root = path.resolve("/tmp/project");

  assert.deepEqual(toProjectRelativeFilePaths(root, [
    path.join(root, "docs", "spec.md"),
    path.join(root, "docs", "spec.md"),
    path.join(root, "src", "main.go"),
  ]), ["docs/spec.md", "src/main.go"]);
});

test("toProjectRelativeFilePaths drops selections outside the project root", () => {
  const root = path.resolve("/tmp/project");

  assert.deepEqual(toProjectRelativeFilePaths(root, [
    path.resolve("/tmp/project-readme.md"),
    path.resolve("/tmp/project", "README.md"),
    path.resolve("/tmp/elsewhere.md"),
  ]), ["README.md"]);
});

test("toProjectAbsolutePath resolves contained project paths", () => {
  const root = path.resolve("/tmp/project");

  assert.equal(
    toProjectAbsolutePath(root, "docs/spec.md"),
    path.resolve(root, "docs", "spec.md"),
  );
});

test("toProjectAbsolutePath rejects empty and escaping paths", () => {
  const root = path.resolve("/tmp/project");

  assert.equal(toProjectAbsolutePath(root, ""), null);
  assert.equal(toProjectAbsolutePath(root, "../outside.md"), null);
  assert.equal(toProjectAbsolutePath(root, path.resolve("/tmp/elsewhere.md")), null);
});
