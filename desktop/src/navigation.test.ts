import assert from "node:assert/strict";
import test from "node:test";

import {
  isSafeExternalURL,
  isTrustedNavigationURL,
  isTrustedRendererURL,
} from "./navigation.js";

const rendererBaseURL = "http://127.0.0.1:5173/dashboard";

test("trusts renderer URLs on the configured origin", () => {
  assert.equal(isTrustedRendererURL("http://127.0.0.1:5173/chains/chain-1", rendererBaseURL), true);
  assert.equal(isTrustedRendererURL("http://127.0.0.1:5174/chains/chain-1", rendererBaseURL), false);
  assert.equal(isTrustedRendererURL("https://127.0.0.1:5173/chains/chain-1", rendererBaseURL), false);
});

test("rejects navigation outside the configured renderer origin", () => {
  assert.equal(isTrustedNavigationURL("http://127.0.0.1:5173/chains/chain-1", rendererBaseURL), true);
  assert.equal(isTrustedNavigationURL("data:text/html;charset=utf-8,Starting", rendererBaseURL), false);
  assert.equal(isTrustedNavigationURL("http://example.test/phish", rendererBaseURL), false);
  assert.equal(isTrustedNavigationURL("javascript:alert(1)", rendererBaseURL), false);
});

test("allows only the app-owned status data URL", () => {
  const statusURL = "data:text/html;charset=utf-8,Starting";
  assert.equal(isTrustedNavigationURL(statusURL, rendererBaseURL, statusURL), true);
  assert.equal(isTrustedNavigationURL("data:text/html;charset=utf-8,Other", rendererBaseURL, statusURL), false);
});

test("classifies safe system-browser external URLs", () => {
  assert.equal(isSafeExternalURL("https://example.test/docs"), true);
  assert.equal(isSafeExternalURL("http://example.test/docs"), true);
  assert.equal(isSafeExternalURL("mailto:support@example.test"), true);
  assert.equal(isSafeExternalURL("file:///tmp/secret.txt"), false);
  assert.equal(isSafeExternalURL("javascript:alert(1)"), false);
});
