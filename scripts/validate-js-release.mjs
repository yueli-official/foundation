import { execFileSync } from "node:child_process";
import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const packagesRoot = join(root, "js", "packages");
const baseRef = process.argv[2]?.trim();

function readPackages() {
  return readdirSync(packagesRoot, { withFileTypes: true })
    .filter((entry) => entry.isDirectory())
    .map((entry) => {
      const directory = join(packagesRoot, entry.name);
      const manifestPath = join(directory, "package.json");
      const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
      return {
        directory,
        relativeDirectory: relative(root, directory).replaceAll("\\", "/"),
        manifest,
      };
    })
    .filter(({ manifest }) => manifest.private !== true)
    .sort((left, right) =>
      left.manifest.name.localeCompare(right.manifest.name),
    );
}

function git(...args) {
  return execFileSync("git", args, { cwd: root, encoding: "utf8" }).trim();
}

function previousManifest(base, packageDirectory) {
  if (!base) return undefined;
  try {
    return JSON.parse(git("show", `${base}:${packageDirectory}/package.json`));
  } catch {
    return undefined;
  }
}

const packages = readPackages();
const failures = [];
for (const { manifest, relativeDirectory } of packages) {
  if (
    typeof manifest.name !== "string" ||
    !manifest.name.startsWith("@yueli/")
  ) {
    failures.push(
      `${relativeDirectory}: package name must use the @yueli scope`,
    );
  }
  if (
    typeof manifest.version !== "string" ||
    !/^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$/.test(manifest.version)
  ) {
    failures.push(
      `${relativeDirectory}: package version must be explicit SemVer`,
    );
  }
  if (!manifest.exports || typeof manifest.exports !== "object") {
    failures.push(
      `${relativeDirectory}: public package must declare explicit exports`,
    );
  }
  if (!Array.isArray(manifest.files) || manifest.files.length === 0) {
    failures.push(
      `${relativeDirectory}: public package must declare a files allowlist`,
    );
  }
}

let changedPackages = packages;
if (baseRef) {
  const changed = new Set(
    git("diff", "--name-only", baseRef, "--", "js/packages")
      .split(/\r?\n/u)
      .filter(Boolean),
  );
  changedPackages = packages.filter(({ relativeDirectory }) =>
    [...changed].some(
      (path) =>
        path === relativeDirectory || path.startsWith(`${relativeDirectory}/`),
    ),
  );
  for (const { manifest, relativeDirectory } of changedPackages) {
    const previous = previousManifest(baseRef, relativeDirectory);
    if (previous?.version === manifest.version) {
      failures.push(
        `${manifest.name}: package content changed since ${baseRef}, but version is still ${manifest.version}`,
      );
    }
  }
}

if (changedPackages.length === 0) {
  failures.push(
    `no public JS package changed${baseRef ? ` since ${baseRef}` : ""}`,
  );
}
if (failures.length > 0) {
  throw new Error(`JS release validation failed:\n- ${failures.join("\n- ")}`);
}

console.log(
  JSON.stringify(
    {
      baseRef: baseRef || null,
      publicPackages: packages.map(({ manifest }) => ({
        name: manifest.name,
        version: manifest.version,
      })),
      changedPackages: changedPackages.map(({ manifest }) => ({
        name: manifest.name,
        version: manifest.version,
      })),
    },
    null,
    2,
  ),
);
