import assert from "node:assert/strict";
import test from "node:test";

import { compareModelRelease, modelCatalog } from "./models.ts";

test("models are ordered by their current release date with aliases last", () => {
  assert.deepEqual(
    modelCatalog.toSorted(compareModelRelease).map((model) => model.name),
    [
      "DeepSeek-V4 Flash",
      "Kimi K3",
      "GLM-5.2",
      "Kimi K2.7 Code",
      "DeepSeek-V4 Pro",
      "Kimi K2.6",
      "GLM-5.1",
      "GLM-5.2 Sale",
      "GLM-5.2 Fast",
    ],
  );
});
