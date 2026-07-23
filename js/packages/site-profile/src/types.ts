export type VisualKind = "icon" | "asset";

export interface SiteVisual {
  kind: VisualKind;
  ref: string;
  alt?: string;
}

export interface SiteLink {
  id: string;
  label: string;
  href: string;
  icon?: string;
}

export interface SiteProfile {
  identity: { name: string; tagline?: string; description?: string };
  branding: {
    logo?: SiteVisual;
    darkLogo?: SiteVisual;
    favicon?: SiteVisual;
  };
  announcement: {
    enabled: boolean;
    text?: string;
    tone?: "neutral" | "info" | "success" | "warning" | "critical";
    action?: SiteLink;
    dismissible: boolean;
    startsAt?: string;
    endsAt?: string;
  };
  support: {
    contacts: Array<{
      id: string;
      kind: "email" | "phone" | "link" | "text";
      label: string;
      value: string;
      icon?: string;
    }>;
  };
  footer: {
    tagline?: string;
    copyright?: string;
    linkGroups: Array<{ id: string; title: string; links: SiteLink[] }>;
    social: Array<{
      id: string;
      platform: string;
      label?: string;
      url: string;
      icon?: string;
    }>;
    legal: SiteLink[];
    compliance: {
      records: Array<{
        id: string;
        kind: string;
        label: string;
        number: string;
        url?: string;
      }>;
      extraText?: string;
    };
  };
}

export interface SiteProfileSnapshot {
  profile: SiteProfile;
  revision: number;
  etag: string;
  documentDigest: string;
  schemaVersion: number;
  updatedAt: string;
}

export interface SiteProfileReplaceResult {
  snapshot: SiteProfileSnapshot;
  changed: boolean;
}

export type FieldControl =
  "text" | "textarea" | "toggle" | "select" | "visual" | "datetime" | "list";

export interface FormField {
  path: string;
  label: string;
  description?: string;
  control: FieldControl;
  required: boolean;
  maxLength?: number;
  maxItems?: number;
  options?: Array<{ value: string; label: string }>;
  itemFields?: FormField[];
}

export interface FormSection {
  id: string;
  label: string;
  description?: string;
  fields: FormField[];
}

export interface SiteProfileFormSchema {
  version: number;
  digest: string;
  sections: FormSection[];
}

export interface ReplaceRequest {
  method: "PUT";
  headers: { "If-Match": string };
  body: { profile: SiteProfile };
}
