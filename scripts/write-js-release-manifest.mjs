import { createHash } from "node:crypto";
import { readdirSync, readFileSync, statSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const output = process.argv[2];
const tag = process.argv[3];
const commit = process.argv[4];
if (!output || !tag || !commit) {
  throw new Error(
    "usage: node scripts/write-js-release-manifest.mjs <output> <tag> <commit>",
  );
}

const packagesRoot = join(root, "js", "packages");
const packages = readdirSync(packagesRoot, { withFileTypes: true })
  .filter((entry) => entry.isDirectory())
  .map((entry) =>
    JSON.parse(
      readFileSync(join(packagesRoot, entry.name, "package.json"), "utf8"),
    ),
  )
  .filter((manifest) => manifest.private !== true)
  .sort((left, right) => left.name.localeCompare(right.name));

const artifacts = packages.map((manifest) => {
  const filename = `${manifest.name.slice(1).replaceAll("/", "-")}-${manifest.version}.tgz`;
  const path = join(output, filename);
  const bytes = readFileSync(path);
  return {
    name: manifest.name,
    version: manifest.version,
    filename,
    bytes: statSync(path).size,
    sha256: createHash("sha256").update(bytes).digest("hex"),
  };
});

writeFileSync(
  join(output, "foundation-js-release-manifest.v1.json"),
  `${JSON.stringify({ schemaVersion: 1, releaseModel: "full-bundle", tag, commit, packages: artifacts }, null, 2)}\n`,
);
writeFileSync(
  join(output, "SHA256SUMS"),
  `${artifacts.map((artifact) => `${artifact.sha256}  ${artifact.filename}`).join("\n")}\n`,
);

console.log(JSON.stringify(artifacts, null, 2));
