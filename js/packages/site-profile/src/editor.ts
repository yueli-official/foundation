import { assertSiteProfileSchema, assertSiteProfileSnapshot } from "./contract";
import type {
  FormField,
  ReplaceRequest,
  SiteProfile,
  SiteProfileFormSchema,
  SiteProfileReplaceResult,
  SiteProfileSnapshot,
} from "./types";

export class SiteProfileEditor {
  readonly schema: SiteProfileFormSchema;
  #snapshot: SiteProfileSnapshot;
  #draft: SiteProfile;
  #baseline: string;
  readonly #fields = new Map<string, FormField>();

  constructor(schema: unknown, snapshot: unknown) {
    assertSiteProfileSchema(schema);
    assertSiteProfileSnapshot(snapshot);
    if (schema.version !== snapshot.schemaVersion) {
      throw new Error(
        `site-profile: schema version ${schema.version} does not match snapshot ${snapshot.schemaVersion}`,
      );
    }
    this.schema = structuredClone(schema);
    this.#snapshot = structuredClone(snapshot);
    this.#draft = structuredClone(snapshot.profile);
    this.#baseline = stableJSON(this.#draft);
    for (const section of this.schema.sections) {
      for (const field of section.fields) this.#fields.set(field.path, field);
    }
  }

  get snapshot(): SiteProfileSnapshot {
    return structuredClone(this.#snapshot);
  }

  get draft(): SiteProfile {
    return this.#draft;
  }

  get dirty(): boolean {
    return stableJSON(this.#draft) !== this.#baseline;
  }

  field(path: string): FormField {
    const field = this.#fields.get(path);
    if (!field) throw new Error(`site-profile: unknown schema field ${path}`);
    return field;
  }

  get(path: string): unknown {
    this.field(path);
    return readPath(this.#draft as unknown as Record<string, unknown>, path);
  }

  set(path: string, value: unknown): void {
    this.field(path);
    writePath(this.#draft as unknown as Record<string, unknown>, path, value);
  }

  replaceDraft(profile: SiteProfile): void {
    this.#draft = structuredClone(profile);
  }

  reset(): void {
    this.#draft = structuredClone(this.#snapshot.profile);
    this.#baseline = stableJSON(this.#draft);
  }

  request(ifMatch = this.#snapshot.etag): ReplaceRequest {
    return {
      method: "PUT",
      headers: { "If-Match": ifMatch },
      body: { profile: structuredClone(this.#draft) },
    };
  }

  apply(result: SiteProfileReplaceResult): void {
    assertSiteProfileSnapshot(result.snapshot);
    if (result.snapshot.schemaVersion !== this.schema.version) {
      throw new Error("site-profile: replacement schema version changed");
    }
    this.#snapshot = structuredClone(result.snapshot);
    this.#draft = structuredClone(result.snapshot.profile);
    this.#baseline = stableJSON(this.#draft);
  }
}

function pathParts(path: string): string[] {
  const parts = path.split(".");
  if (
    parts.some(
      (part) =>
        part === "__proto__" || part === "prototype" || part === "constructor",
    )
  ) {
    throw new Error(`site-profile: unsafe field path ${path}`);
  }
  return parts;
}

function readPath(root: Record<string, unknown>, path: string): unknown {
  let current: unknown = root;
  for (const part of pathParts(path)) {
    if (
      current === null ||
      typeof current !== "object" ||
      Array.isArray(current)
    )
      return undefined;
    current = (current as Record<string, unknown>)[part];
  }
  return current;
}

function writePath(
  root: Record<string, unknown>,
  path: string,
  value: unknown,
): void {
  const parts = pathParts(path);
  let current = root;
  for (const part of parts.slice(0, -1)) {
    const next = current[part];
    if (next === null || typeof next !== "object" || Array.isArray(next)) {
      throw new Error(`site-profile: field parent does not exist for ${path}`);
    }
    current = next as Record<string, unknown>;
  }
  current[parts.at(-1)!] = value;
}

function stableJSON(value: unknown): string {
  return JSON.stringify(value);
}
