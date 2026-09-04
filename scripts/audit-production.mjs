import { spawnSync } from "node:child_process";

const attempts = 3;
const pnpmEntry = process.env.npm_execpath;
const command = pnpmEntry ? process.execPath : "pnpm";
const commandPrefix = pnpmEntry ? [pnpmEntry] : [];
const retryable =
  /(?:ERR_SOCKET_TIMEOUT|ETIMEDOUT|ECONNRESET|EAI_AGAIN|ENETUNREACH|fetch failed|ERR_PNPM_AUDIT_BAD_RESPONSE[^\n]*(?:500|502|503|504))/iu;

for (let attempt = 1; attempt <= attempts; attempt += 1) {
  const result = spawnSync(
    command,
    [...commandPrefix, "audit", "--prod", "--audit-level", "moderate"],
    {
      encoding: "utf8",
      env: {
        ...process.env,
        npm_config_fetch_retries: "0",
        npm_config_fetch_timeout: "30000",
      },
    },
  );

  process.stdout.write(result.stdout ?? "");
  process.stderr.write(result.stderr ?? "");
  if (result.error) console.error(result.error);

  if (result.status === 0) process.exit(0);

  const output = `${result.stdout ?? ""}\n${result.stderr ?? ""}`;
  if (!retryable.test(output) || attempt === attempts) {
    process.exit(result.status ?? 1);
  }

  const delaySeconds = attempt * 10;
  console.warn(
    `Dependency audit endpoint unavailable (attempt ${attempt}/${attempts}); retrying in ${delaySeconds}s.`,
  );
  await new Promise((resolve) => setTimeout(resolve, delaySeconds * 1000));
}
