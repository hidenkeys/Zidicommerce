import test from "node:test";
import assert from "node:assert/strict";
import { creationModes, experienceModes, experienceModeLabel, experienceStatus, scenarioLabel, previewScenarios } from "../src/lib/experience-modes.ts";

test("four creation choices retain exact backend identifiers and distinct presentation", () => {
  assert.deepEqual(creationModes.map(mode => mode.id), ["automated_commerce", "guided_questions", "whatsapp_flow", "ai"]);
  assert.equal(new Set(creationModes.map(mode => mode.icon)).size, 4);
  for (const mode of creationModes) assert.ok(mode.label && mode.description && mode.bestFor && mode.accent);
});
test("legacy journeys keep display support without becoming a creation choice", () => {
  assert.equal(experienceModeLabel("guided_chat"), "Legacy guided chat");
  assert.equal(experienceModes.guided_chat.id, "guided_chat");
  assert.equal(experienceModeLabel("future_mode"), "future mode");
  assert.equal(experienceModeLabel(), "Experience");
});
test("status reflects bindings rather than publication alone and archive takes precedence", () => {
  assert.equal(experienceStatus({status:"draft"}), "Draft");
  assert.equal(experienceStatus({status:"published",published_version_id:"published"}), "Draft");
  assert.equal(experienceStatus({status:"published"},{paused:false}), "Live");
  assert.equal(experienceStatus({status:"published"},{paused:true}), "Paused");
  assert.equal(experienceStatus({status:"archived"},{paused:false}), "Archived");
  assert.equal(experienceStatus({status:"archived"},{paused:true}), "Archived");
});
test("preview labels are human readable and preserve request values", () => {
  assert.equal(scenarioLabel("normal"), "Happy path");
  assert.equal(scenarioLabel("empty_catalogue"), "Empty catalogue");
  assert.deepEqual(previewScenarios.map(item => item.value), ["normal", "empty_catalogue", "unavailable_stock", "payment_failure", "handoff", "unsupported_channel"]);
  assert.equal(scenarioLabel("future_case"), "future case");
});
