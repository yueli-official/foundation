import type {
  FormField,
  SiteProfileFormSchema,
  SiteProfileSnapshot,
} from "./types";

const digestPattern = /^[0-9a-f]{64}$/u;
const pathPattern = /^[A-Za-z][A-Za-z0-9]*(?:\.[A-Za-z][A-Za-z0-9]*)*$/u;
const controls = new Set([
  "text",
  "textarea",
  "toggle",
  "select",
  "visual",
  "datetime",
  "list",
]);

export class SiteProfileContractError extends Error {
  constructor(
    readonly code: string,
    readonly path: string,
    message: string,
  ) {
    super(`site-profile: ${code} at ${path}: ${message}`);
    this.name = "SiteProfileContractError";
  }
}

function fail(code: string, path: string, message: string): never {
  throw new SiteProfileContractError(code, path, message);
}

function objectAt(value: unknown, path: string): Record<string, unknown> {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    fail("object_required", path, "must be an object");
  }
  return value as Record<string, unknown>;
}

function stringAt(value: unknown, path: string): string {
  if (typeof value !== "string" || value.length === 0) {
    fail("string_required", path, "must be a non-empty string");
  }
  return value;
}

function positiveIntegerAt(value: unknown, path: string): number {
  if (!Number.isSafeInteger(value) || Number(value) < 1) {
    fail("positive_integer_required", path, "must be a positive integer");
  }
  return Number(value);
}

export function assertSiteProfileSchema(
  value: unknown,
): asserts value is SiteProfileFormSchema {
  const schema = objectAt(value, "$");
  positiveIntegerAt(schema.version, "$.version");
  if (!digestPattern.test(stringAt(schema.digest, "$.digest"))) {
    fail("digest_invalid", "$.digest", "must be a lowercase SHA-256 digest");
  }
  if (!Array.isArray(schema.sections) || schema.sections.length === 0) {
    fail("sections_required", "$.sections", "must be a non-empty array");
  }
  const sectionIDs = new Set<string>();
  const fieldPaths = new Set<string>();
  for (const [sectionIndex, rawSection] of schema.sections.entries()) {
    const path = `$.sections[${sectionIndex}]`;
    const section = objectAt(rawSection, path);
    const id = stringAt(section.id, `${path}.id`);
    if (sectionIDs.has(id))
      fail("id_duplicate", `${path}.id`, "must be unique");
    sectionIDs.add(id);
    stringAt(section.label, `${path}.label`);
    if (!Array.isArray(section.fields) || section.fields.length === 0) {
      fail("fields_required", `${path}.fields`, "must be a non-empty array");
    }
    for (const [fieldIndex, rawField] of section.fields.entries()) {
      assertField(rawField, `${path}.fields[${fieldIndex}]`, fieldPaths, true);
    }
  }
}

function assertField(
  value: unknown,
  path: string,
  seen: Set<string>,
  topLevel: boolean,
): asserts value is FormField {
  const field = objectAt(value, path);
  const fieldPath = stringAt(field.path, `${path}.path`);
  if (!pathPattern.test(fieldPath)) {
    fail("path_invalid", `${path}.path`, "must be a safe dotted property path");
  }
  if (topLevel && seen.has(fieldPath)) {
    fail("path_duplicate", `${path}.path`, "must be unique");
  }
  if (topLevel) seen.add(fieldPath);
  stringAt(field.label, `${path}.label`);
  if (!controls.has(String(field.control))) {
    fail("control_invalid", `${path}.control`, "is unsupported");
  }
  if (typeof field.required !== "boolean") {
    fail("boolean_required", `${path}.required`, "must be a boolean");
  }
  if (field.itemFields !== undefined) {
    if (!Array.isArray(field.itemFields)) {
      fail("array_required", `${path}.itemFields`, "must be an array");
    }
    for (const [index, item] of field.itemFields.entries()) {
      assertField(item, `${path}.itemFields[${index}]`, new Set(), false);
    }
  }
}

export function assertSiteProfileSnapshot(
  value: unknown,
): asserts value is SiteProfileSnapshot {
  const snapshot = objectAt(value, "$");
  positiveIntegerAt(snapshot.revision, "$.revision");
  positiveIntegerAt(snapshot.schemaVersion, "$.schemaVersion");
  if (
    !/^"site-profile-v\d+-r\d+-[0-9a-f]{16}"$/u.test(
      stringAt(snapshot.etag, "$.etag"),
    )
  ) {
    fail("etag_invalid", "$.etag", "must be a strong Site Profile ETag");
  }
  if (
    !digestPattern.test(stringAt(snapshot.documentDigest, "$.documentDigest"))
  ) {
    fail(
      "digest_invalid",
      "$.documentDigest",
      "must be a lowercase SHA-256 digest",
    );
  }
  if (Number.isNaN(Date.parse(stringAt(snapshot.updatedAt, "$.updatedAt")))) {
    fail("datetime_invalid", "$.updatedAt", "must be an RFC3339 timestamp");
  }
  const profile = objectAt(snapshot.profile, "$.profile");
  objectAt(profile.identity, "$.profile.identity");
  objectAt(profile.branding, "$.profile.branding");
  objectAt(profile.announcement, "$.profile.announcement");
  objectAt(profile.support, "$.profile.support");
  objectAt(profile.footer, "$.profile.footer");
}
