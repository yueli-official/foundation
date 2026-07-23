import {
  discoveryContractVersion,
  type DiscoveryHeadInput,
  type DiscoveryProjection,
} from "./types";

export class DiscoveryContractError extends Error {
  readonly code: string;
  readonly path: string;

  constructor(code: string, path: string, message: string) {
    super(`discovery: ${code} at ${path}: ${message}`);
    this.name = "DiscoveryContractError";
    this.code = code;
    this.path = path;
  }
}

function fail(code: string, path: string, message: string): never {
  throw new DiscoveryContractError(code, path, message);
}

function objectAt(
  value: unknown,
  path: string,
): Readonly<Record<string, unknown>> {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    fail("object_required", path, "must be an object");
  }
  return value as Readonly<Record<string, unknown>>;
}

function stringAt(value: unknown, path: string, allowEmpty = false): string {
  if (typeof value !== "string" || (!allowEmpty && value.length === 0)) {
    fail("string_required", path, "must be a non-empty string");
  }
  return value;
}

function arrayAt(value: unknown, path: string): readonly unknown[] {
  if (!Array.isArray(value)) {
    fail("array_required", path, "must be an array");
  }
  return value;
}

function httpURL(value: unknown, path: string): string {
  const text = stringAt(value, path);
  let url: URL;
  try {
    url = new URL(text);
  } catch {
    fail("invalid_url", path, "must be an absolute URL");
  }
  if (
    !["http:", "https:"].includes(url.protocol) ||
    url.username !== "" ||
    url.password !== "" ||
    url.hash !== ""
  ) {
    fail(
      "invalid_url",
      path,
      "must be an HTTP(S) URL without credentials or fragment",
    );
  }
  return url.href;
}

export function assertDiscoveryProjection(
  value: unknown,
): asserts value is DiscoveryProjection {
  const projection = objectAt(value, "$");
  if (projection.contractVersion !== discoveryContractVersion) {
    fail(
      "unsupported_contract_version",
      "$.contractVersion",
      `must equal ${discoveryContractVersion}`,
    );
  }
  stringAt(projection.key, "$.key");
  const canonical = httpURL(projection.canonicalUrl, "$.canonicalUrl");
  const head = objectAt(projection.head, "$.head");
  stringAt(head.title, "$.head.title");
  const links = arrayAt(head.links, "$.head.links");
  const canonicalLinks: string[] = [];
  for (const [index, item] of links.entries()) {
    const link = objectAt(item, `$.head.links[${index}]`);
    const rel = stringAt(link.rel, `$.head.links[${index}].rel`);
    const href = httpURL(link.href, `$.head.links[${index}].href`);
    if (rel === "canonical") {
      canonicalLinks.push(href);
    }
    if (link.hreflang !== undefined) {
      stringAt(link.hreflang, `$.head.links[${index}].hreflang`);
    }
  }
  if (canonicalLinks.length !== 1 || canonicalLinks[0] !== canonical) {
    fail(
      "canonical_drift",
      "$.head.links",
      "must contain exactly one canonical link matching canonicalUrl",
    );
  }
  const meta = arrayAt(head.meta, "$.head.meta");
  let robots = "";
  let openGraphURL = "";
  for (const [index, item] of meta.entries()) {
    const tag = objectAt(item, `$.head.meta[${index}]`);
    const name =
      tag.name === undefined
        ? undefined
        : stringAt(tag.name, `$.head.meta[${index}].name`);
    const property =
      tag.property === undefined
        ? undefined
        : stringAt(tag.property, `$.head.meta[${index}].property`);
    if ((name === undefined) === (property === undefined)) {
      fail(
        "meta_identity_required",
        `$.head.meta[${index}]`,
        "must contain exactly one of name or property",
      );
    }
    const content = stringAt(
      tag.content,
      `$.head.meta[${index}].content`,
      true,
    );
    if (name === "robots") {
      robots = content;
    }
    if (property === "og:url") {
      openGraphURL = httpURL(content, `$.head.meta[${index}].content`);
    }
  }
  if (openGraphURL !== "" && openGraphURL !== canonical) {
    fail("canonical_drift", "$.head.meta", "og:url must match canonicalUrl");
  }
  const structuredData = arrayAt(head.structuredData, "$.head.structuredData");
  for (const [index, item] of structuredData.entries()) {
    const block = objectAt(item, `$.head.structuredData[${index}]`);
    stringAt(block.id, `$.head.structuredData[${index}].id`);
    objectAt(block.json, `$.head.structuredData[${index}].json`);
  }
  const headers = objectAt(projection.headers, "$.headers");
  if (headers.xRobotsTag !== undefined) {
    stringAt(headers.xRobotsTag, "$.headers.xRobotsTag");
  }
  if (headers.link !== undefined) {
    for (const [index, item] of arrayAt(
      headers.link,
      "$.headers.link",
    ).entries()) {
      const header = stringAt(item, `$.headers.link[${index}]`);
      if (/[\r\n\0]/u.test(header)) {
        fail(
          "unsafe_header",
          `$.headers.link[${index}]`,
          "must not contain control line breaks",
        );
      }
    }
  }
  if (projection.sitemap !== undefined) {
    const sitemap = objectAt(projection.sitemap, "$.sitemap");
    const location = httpURL(sitemap.location, "$.sitemap.location");
    if (location !== canonical) {
      fail("canonical_drift", "$.sitemap.location", "must match canonicalUrl");
    }
    if (robots.toLowerCase().includes("noindex")) {
      fail(
        "noindex_sitemap_conflict",
        "$.sitemap",
        "a noindex projection cannot enter a sitemap",
      );
    }
  }
  arrayAt(projection.diagnostics, "$.diagnostics");
}

function safeJSON(value: Readonly<Record<string, unknown>>): string {
  return JSON.stringify(value)
    .replaceAll("<", "\\u003c")
    .replaceAll(">", "\\u003e")
    .replaceAll("&", "\\u0026")
    .replaceAll("\u2028", "\\u2028")
    .replaceAll("\u2029", "\\u2029");
}

export function toDiscoveryHead(value: unknown): DiscoveryHeadInput {
  assertDiscoveryProjection(value);
  return {
    title: value.head.title,
    titleTemplate: null,
    link: value.head.links.map((link, index) => ({
      key: `discovery:link:${index}:${link.rel}:${link.hreflang ?? ""}`,
      ...link,
    })),
    meta: value.head.meta.map((meta, index) => ({
      key: `discovery:meta:${index}:${meta.name ?? meta.property ?? ""}`,
      ...meta,
    })),
    script: value.head.structuredData.map((block, index) => ({
      key: `discovery:jsonld:${index}:${block.id}`,
      id: block.id,
      type: "application/ld+json",
      innerHTML: safeJSON(block.json),
    })),
  };
}
