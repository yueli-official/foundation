import { createHash } from "node:crypto";
import { createError, setHeader, type H3Event } from "h3";
import { useRuntimeConfig } from "#imports";

interface DiscoveryArtifact {
  name: string;
  mediaType: string;
  content: string;
  bytes: number;
  sha256: string;
}

interface DiscoveryArtifactResult {
  contractVersion: "discovery.v1";
  artifact?: DiscoveryArtifact;
}

interface DiscoveryArtifactResponse {
  result?: DiscoveryArtifactResult;
  data?: { result?: DiscoveryArtifactResult };
}

export async function serveDiscoveryArtifact(
  event: H3Event,
  name: string,
): Promise<string> {
  if (
    name.length === 0 ||
    name.includes("\\") ||
    name.includes("..") ||
    name.startsWith("/")
  ) {
    throw createError({
      statusCode: 400,
      statusMessage: "Invalid discovery artifact",
    });
  }
  const apiBase = useRuntimeConfig(event).apiBase as string;
  const response = await $fetch<DiscoveryArtifactResponse>(
    `${apiBase}/api/v1/discovery/artifact`,
    { query: { name } },
  );
  const result = response.result ?? response.data?.result;
  if (result?.contractVersion !== "discovery.v1") {
    throw createError({
      statusCode: 502,
      statusMessage: "Unsupported discovery publication",
    });
  }
  const artifact = result.artifact;
  if (artifact === undefined) {
    throw createError({
      statusCode: 404,
      statusMessage: "Discovery artifact not found",
    });
  }
  const encodedBytes = new TextEncoder().encode(artifact.content).byteLength;
  if (encodedBytes !== artifact.bytes) {
    throw createError({
      statusCode: 502,
      statusMessage: "Invalid discovery artifact length",
    });
  }
  const digest = createHash("sha256")
    .update(artifact.content, "utf8")
    .digest("hex");
  if (digest !== artifact.sha256.toLowerCase()) {
    throw createError({
      statusCode: 502,
      statusMessage: "Invalid discovery artifact digest",
    });
  }
  setHeader(event, "content-type", artifact.mediaType);
  setHeader(event, "etag", `"sha256-${artifact.sha256}"`);
  setHeader(event, "cache-control", "public, max-age=60, stale-if-error=300");
  return artifact.content;
}
