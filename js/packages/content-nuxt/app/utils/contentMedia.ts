const MEDIA_PATH = /\/media\/[0-9A-Za-z_-]{1,64}$/;
const RENDITION = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;

export type ContentImageFormat = "jpg" | "png" | "webp";

// Re-targets an Asset stable media URL to another accepted named rendition.
// Non-Asset images are deliberately left alone so external Markdown behaves
// like ordinary content instead of being routed through Asset by accident.
export function contentAssetRenditionURL(
  source: string,
  rendition: string,
  format: ContentImageFormat = "webp",
): string {
  if (!source || !RENDITION.test(rendition)) return "";
  const relative = source.startsWith("/");
  let parsed: URL;
  try {
    parsed = new URL(source, "https://content.yueli.invalid");
  } catch {
    return "";
  }
  if (!MEDIA_PATH.test(parsed.pathname)) return "";
  parsed.searchParams.set("format", format);
  parsed.searchParams.set("name", rendition);
  return relative
    ? `${parsed.pathname}${parsed.search}${parsed.hash}`
    : parsed.toString();
}
