export const discoveryContractVersion = "discovery.v1" as const;

export interface DiscoveryDiagnostic {
  code: string;
  severity: "info" | "warning" | "error";
  path?: string;
  protocol?: string;
  reference?: string;
  message: string;
}

export interface DiscoveryLink {
  rel: string;
  href: string;
  hreflang?: string;
}

export interface DiscoveryMeta {
  name?: string;
  property?: string;
  content: string;
}

export interface DiscoveryStructuredData {
  id: string;
  json: Readonly<Record<string, unknown>>;
}

export interface DiscoveryProjection {
  contractVersion: typeof discoveryContractVersion;
  key: string;
  canonicalUrl: string;
  head: {
    title: string;
    links: readonly DiscoveryLink[];
    meta: readonly DiscoveryMeta[];
    structuredData: readonly DiscoveryStructuredData[];
  };
  headers: {
    link?: readonly string[];
    xRobotsTag?: string;
  };
  sitemap?: {
    location: string;
    lastModified?: string;
    alternates?: readonly {
      locale: string;
      location: string;
    }[];
  };
  diagnostics: readonly DiscoveryDiagnostic[];
}

export interface DiscoveryHeadInput {
  title: string;
  titleTemplate: null;
  link: readonly (DiscoveryLink & { key: string })[];
  meta: readonly (DiscoveryMeta & { key: string })[];
  script: readonly {
    key: string;
    id: string;
    type: "application/ld+json";
    innerHTML: string;
  }[];
}
