// Layer a consumer's config over the shared preset with Renovate's own merge,
// then print the labels Renovate would put on a pull request. Runs inside the
// pinned Renovate image: renovate-labels.bats sends it in with the preset.
import fs from "node:fs";
import { createRequire } from "node:module";
import { mergeChildConfig } from "/usr/local/renovate/dist/config/utils.js";
import { setPlatformApi } from "/usr/local/renovate/dist/modules/platform/index.js";
import { prepareLabels } from "/usr/local/renovate/dist/workers/repository/update/pr/labels.js";

const JSON5 = createRequire("/usr/local/renovate/").call(null, "json5");
// prepareLabels asks the platform for its label length limit.
setPlatformApi("local");
const preset = JSON5.parse(fs.readFileSync(process.argv[2], "utf8"));
const consumer = process.env.CONSUMER_LABELS
  ? { labels: JSON.parse(process.env.CONSUMER_LABELS) }
  : {};
console.log(
  `PLANNED_LABELS ${JSON.stringify(prepareLabels(mergeChildConfig(preset, consumer)))}`,
);
