import { describe, expect, it } from "vitest";
import { SiteProfileContractError, SiteProfileEditor } from "../src";
import type {
  SiteProfile,
  SiteProfileFormSchema,
  SiteProfileSnapshot,
} from "../src";

const schema: SiteProfileFormSchema = {
  version: 1,
  digest: "a".repeat(64),
  sections: [
    {
      id: "identity",
      label: "Identity",
      fields: [
        {
          path: "identity.name",
          label: "Name",
          control: "text",
          required: true,
          maxLength: 120,
        },
      ],
    },
  ],
};

const profile: SiteProfile = {
  identity: { name: "Example", tagline: "Tagline" },
  branding: {},
  announcement: { enabled: false, dismissible: false },
  support: { contacts: [] },
  footer: {
    linkGroups: [],
    social: [],
    legal: [],
    compliance: { records: [] },
  },
};

function snapshot(): SiteProfileSnapshot {
  return {
    profile: structuredClone(profile),
    revision: 1,
    etag: `"site-profile-v1-r1-${"b".repeat(16)}"`,
    documentDigest: "b".repeat(64),
    schemaVersion: 1,
    updatedAt: "2026-07-23T12:00:00Z",
  };
}

describe("SiteProfileEditor", () => {
  it("drives fields, dirty/reset and conditional replacement", () => {
    const editor = new SiteProfileEditor(schema, snapshot());
    expect(editor.get("identity.name")).toBe("Example");
    editor.set("identity.name", "Changed");
    expect(editor.dirty).toBe(true);
    expect(editor.request()).toMatchObject({
      method: "PUT",
      headers: { "If-Match": snapshot().etag },
      body: { profile: { identity: { name: "Changed" } } },
    });
    editor.reset();
    expect(editor.dirty).toBe(false);
    expect(editor.get("identity.name")).toBe("Example");
  });

  it("replaces the complete draft while retaining snapshot concurrency metadata", () => {
    const current = snapshot();
    const editor = new SiteProfileEditor(schema, current);
    const replacement = structuredClone(current.profile);
    replacement.identity.name = "Replacement";

    editor.replaceDraft(replacement);

    expect(editor.dirty).toBe(true);
    expect(editor.request()).toEqual({
      method: "PUT",
      headers: { "If-Match": current.etag },
      body: { profile: replacement },
    });
    expect(editor.request('"consumer-etag"').headers["If-Match"]).toBe(
      '"consumer-etag"',
    );
  });

  it("applies a server result as the new baseline", () => {
    const editor = new SiteProfileEditor(schema, snapshot());
    const next = snapshot();
    next.revision = 2;
    next.etag = `"site-profile-v1-r2-${"c".repeat(16)}"`;
    next.documentDigest = "c".repeat(64);
    next.profile.identity.name = "Server";
    editor.apply({ snapshot: next, changed: true });
    expect(editor.get("identity.name")).toBe("Server");
    expect(editor.dirty).toBe(false);
    expect(editor.request().headers["If-Match"]).toBe(next.etag);
  });

  it("fails closed for schema drift and unsafe paths", () => {
    expect(
      () =>
        new SiteProfileEditor(
          {
            ...schema,
            sections: [
              {
                id: "bad",
                label: "Bad",
                fields: [
                  {
                    path: "__proto__.polluted",
                    label: "Bad",
                    control: "text",
                    required: false,
                  },
                ],
              },
            ],
          },
          snapshot(),
        ),
    ).toThrow(SiteProfileContractError);
    expect(
      () => new SiteProfileEditor({ ...schema, version: 2 }, snapshot()),
    ).toThrow(/does not match/u);
  });
});
