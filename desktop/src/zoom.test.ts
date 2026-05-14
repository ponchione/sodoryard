import assert from "node:assert/strict";
import test from "node:test";

import {
  defaultZoomFactor,
  nextZoomFactor,
  normalizeZoomFactor,
  zoomCommandForInput,
} from "./zoom.js";

const baseInput = {
  type: "keyDown",
  key: "=",
  code: "Equal",
  control: true,
  meta: false,
  alt: false,
};

test("zoomCommandForInput recognizes standard zoom shortcuts", () => {
  assert.equal(zoomCommandForInput(baseInput), "in");
  assert.equal(zoomCommandForInput({ ...baseInput, key: "+", code: "Equal", control: false, meta: true }), "in");
  assert.equal(zoomCommandForInput({ ...baseInput, key: "-", code: "Minus" }), "out");
  assert.equal(zoomCommandForInput({ ...baseInput, key: "0", code: "Digit0" }), "reset");
  assert.equal(zoomCommandForInput({ ...baseInput, key: "+", code: "NumpadAdd" }), "in");
  assert.equal(zoomCommandForInput({ ...baseInput, key: "-", code: "NumpadSubtract" }), "out");
  assert.equal(zoomCommandForInput({ ...baseInput, key: "0", code: "Numpad0" }), "reset");
});

test("zoomCommandForInput ignores non-shortcut input", () => {
  assert.equal(zoomCommandForInput({ ...baseInput, type: "keyUp" }), null);
  assert.equal(zoomCommandForInput({ ...baseInput, control: false, meta: false }), null);
  assert.equal(zoomCommandForInput({ ...baseInput, alt: true }), null);
  assert.equal(zoomCommandForInput({ ...baseInput, key: "p", code: "KeyP" }), null);
});

test("zoom factor helpers round, clamp, and reset", () => {
  assert.equal(normalizeZoomFactor(1.234), 1.23);
  assert.equal(normalizeZoomFactor(0.1), 0.7);
  assert.equal(normalizeZoomFactor(9), 1.8);
  assert.equal(normalizeZoomFactor("wide"), undefined);
  assert.equal(nextZoomFactor(1, "in"), 1.1);
  assert.equal(nextZoomFactor(1, "out"), 0.9);
  assert.equal(nextZoomFactor(1.79, "in"), 1.8);
  assert.equal(nextZoomFactor(0.71, "out"), 0.7);
  assert.equal(nextZoomFactor(1.4, "reset"), defaultZoomFactor);
});
