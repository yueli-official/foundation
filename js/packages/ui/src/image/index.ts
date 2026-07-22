export type ImageOutputType = "image/jpeg" | "image/webp" | "image/png";

export interface ImageFileDescriptor {
  readonly name: string;
  readonly type: string;
  readonly size: number;
}

export interface ImageOptimizationOptions {
  readonly enabled?: boolean;
  readonly outputType?: ImageOutputType;
  readonly maxSide?: number;
  readonly maxPixels?: number;
  readonly quality?: number;
  readonly skipGif?: boolean;
  readonly skipSmallWebpBytes?: number;
  readonly discardLarger?: boolean;
}

export interface NormalizedImageOptimizationOptions {
  readonly enabled: boolean;
  readonly outputType: ImageOutputType;
  readonly maxSide: number;
  readonly maxPixels: number;
  readonly quality: number;
  readonly skipGif: boolean;
  readonly skipSmallWebpBytes: number;
  readonly discardLarger: boolean;
}

export type ImageOptimizationDecisionReason =
  | "eligible"
  | "disabled"
  | "unsupported-type"
  | "animated-image"
  | "already-efficient";

export type ImageOptimizationDecision =
  | {
      readonly optimize: true;
      readonly reason: "eligible";
      readonly options: NormalizedImageOptimizationOptions;
    }
  | {
      readonly optimize: false;
      readonly reason: Exclude<ImageOptimizationDecisionReason, "eligible">;
      readonly options: NormalizedImageOptimizationOptions;
    };

const SUPPORTED_INPUT_TYPES = new Set([
  "image/jpeg",
  "image/png",
  "image/webp",
  "image/gif",
]);

function finiteNumber(
  value: number | undefined,
  fallback: number,
  min: number,
  max: number,
) {
  if (value === undefined || !Number.isFinite(value)) return fallback;
  return Math.min(max, Math.max(min, value));
}

export function normalizeImageOptimizationOptions(
  options: ImageOptimizationOptions = {},
): NormalizedImageOptimizationOptions {
  return {
    enabled: options.enabled !== false,
    outputType: options.outputType ?? "image/webp",
    maxSide: Math.round(finiteNumber(options.maxSide, 1920, 320, 8192)),
    maxPixels: Math.round(
      finiteNumber(options.maxPixels, 16_777_216, 1_000_000, 67_108_864),
    ),
    quality: finiteNumber(options.quality, 0.86, 0.1, 1),
    skipGif: options.skipGif !== false,
    skipSmallWebpBytes: Math.round(
      finiteNumber(
        options.skipSmallWebpBytes,
        1024 * 1024,
        0,
        64 * 1024 * 1024,
      ),
    ),
    discardLarger: options.discardLarger !== false,
  };
}

export function evaluateImageOptimization(
  file: ImageFileDescriptor,
  options: ImageOptimizationOptions = {},
): ImageOptimizationDecision {
  const normalized = normalizeImageOptimizationOptions(options);
  if (!normalized.enabled)
    return { optimize: false, reason: "disabled", options: normalized };
  if (!SUPPORTED_INPUT_TYPES.has(file.type))
    return {
      optimize: false,
      reason: "unsupported-type",
      options: normalized,
    };
  if (file.type === "image/gif" && normalized.skipGif)
    return {
      optimize: false,
      reason: "animated-image",
      options: normalized,
    };
  if (file.type === "image/webp" && file.size <= normalized.skipSmallWebpBytes)
    return {
      optimize: false,
      reason: "already-efficient",
      options: normalized,
    };
  return { optimize: true, reason: "eligible", options: normalized };
}

export function shouldOptimizeImage(
  file: ImageFileDescriptor,
  options: ImageOptimizationOptions = {},
) {
  return evaluateImageOptimization(file, options).optimize;
}

export function optimizedImageFilename(
  filename: string,
  outputType: ImageOutputType = "image/webp",
) {
  const extension =
    outputType === "image/jpeg"
      ? "jpg"
      : outputType === "image/png"
        ? "png"
        : "webp";
  const base = filename.replace(/\.[^.]+$/u, "") || "image";
  return `${base}.${extension}`;
}

export interface ImageDimensions {
  readonly width: number;
  readonly height: number;
}

export function calculateContainedImageSize(
  source: ImageDimensions,
  constraints: Pick<
    NormalizedImageOptimizationOptions,
    "maxSide" | "maxPixels"
  >,
): ImageDimensions | null {
  if (
    !Number.isFinite(source.width) ||
    !Number.isFinite(source.height) ||
    source.width <= 0 ||
    source.height <= 0
  )
    return null;

  const maxDimension = Math.max(source.width, source.height);
  const sourcePixels = source.width * source.height;
  const sideScale = constraints.maxSide / maxDimension;
  const pixelScale = Math.sqrt(constraints.maxPixels / sourcePixels);
  const scale = Math.min(1, sideScale, pixelScale);

  return {
    width: Math.max(1, Math.round(source.width * scale)),
    height: Math.max(1, Math.round(source.height * scale)),
  };
}

export type CropShape = "rect" | "circle";
export type CropMode = "fixed" | "free" | "fixed-size" | "fixed-aspect";
export type CropOutputType = ImageOutputType;

export interface CropperConfigInput {
  readonly shape?: CropShape;
  readonly mode?: CropMode;
  readonly aspectRatio?: number;
  readonly outputWidth?: number;
  readonly outputHeight?: number;
  readonly minCropWidth?: number;
  readonly minCropHeight?: number;
}

export interface CropperOutputInput {
  readonly outputWidth?: number;
  readonly outputHeight?: number;
  readonly outputType?: CropOutputType;
  readonly shape?: CropShape;
}

export interface CropperJsConfig {
  readonly options: {
    readonly viewMode: 0 | 1 | 2 | 3;
    readonly dragMode: "crop" | "move" | "none";
    readonly aspectRatio?: number;
    readonly autoCropArea: number;
    readonly background: boolean;
    readonly center: boolean;
    readonly checkCrossOrigin: boolean;
    readonly checkOrientation: boolean;
    readonly cropBoxMovable: boolean;
    readonly cropBoxResizable: boolean;
    readonly guides: boolean;
    readonly highlight: boolean;
    readonly minCropBoxWidth?: number;
    readonly minCropBoxHeight?: number;
    readonly modal: boolean;
    readonly movable: boolean;
    readonly responsive: boolean;
    readonly rotatable: boolean;
    readonly scalable: boolean;
    readonly toggleDragModeOnDblclick: boolean;
    readonly zoomOnTouch: boolean;
    readonly zoomOnWheel: boolean;
    readonly zoomable: boolean;
  };
  readonly presetMode?: never;
}

export function normalizeCropMode(
  mode: CropMode | undefined,
  hasFixedOutput: boolean,
) {
  if (mode === "free") return "free" as const;
  if (mode === "fixed-aspect") return "fixed-aspect" as const;
  if (mode === "fixed-size") return "fixed-size" as const;
  return hasFixedOutput ? ("fixed-size" as const) : ("fixed-aspect" as const);
}

function positiveOrUndefined(value: number | undefined) {
  return value !== undefined && Number.isFinite(value) && value > 0
    ? value
    : undefined;
}

export function createCropperJsConfig(
  input: CropperConfigInput,
): CropperJsConfig {
  const shape = input.shape ?? "rect";
  const outputWidth = positiveOrUndefined(input.outputWidth);
  const outputHeight = positiveOrUndefined(input.outputHeight);
  const mode = normalizeCropMode(
    input.mode,
    outputWidth !== undefined && outputHeight !== undefined,
  );
  const requestedAspect = positiveOrUndefined(input.aspectRatio);
  const aspectRatio =
    shape === "circle" ? 1 : mode === "free" ? undefined : requestedAspect;

  return {
    options: {
      viewMode: 1,
      dragMode: "move",
      aspectRatio,
      autoCropArea: 0.92,
      background: false,
      center: true,
      checkCrossOrigin: true,
      checkOrientation: true,
      cropBoxMovable: true,
      cropBoxResizable: true,
      guides: true,
      highlight: true,
      minCropBoxWidth: positiveOrUndefined(input.minCropWidth),
      minCropBoxHeight: positiveOrUndefined(input.minCropHeight),
      modal: true,
      movable: true,
      responsive: true,
      rotatable: true,
      scalable: true,
      toggleDragModeOnDblclick: false,
      zoomOnTouch: true,
      zoomOnWheel: true,
      zoomable: true,
    },
  };
}

export function createCropperOutputOptions(input: CropperOutputInput) {
  const width = positiveOrUndefined(input.outputWidth);
  const height = positiveOrUndefined(input.outputHeight);
  return {
    ...(width ? { width } : {}),
    ...(height ? { height } : {}),
    ...(input.shape === "circle" ? { rounded: true } : {}),
    ...(input.outputType === "image/jpeg" ? { fillColor: "#fff" } : {}),
    imageSmoothingQuality: "high" as const,
  };
}

export function extensionForCrop(type: CropOutputType) {
  if (type === "image/png") return "png";
  if (type === "image/jpeg") return "jpg";
  return "webp";
}

export function cropFilename(name: string, type: CropOutputType) {
  const baseName = name.replace(/\.[^.]+$/u, "") || "image";
  return `${baseName}-crop.${extensionForCrop(type)}`;
}

export interface DimensionBounds {
  readonly minWidth?: number;
  readonly minHeight?: number;
  readonly maxWidth?: number;
  readonly maxHeight?: number;
}

export interface DimensionViolation {
  readonly axis: "width" | "height";
  readonly bound: "min" | "max";
  readonly actual: number;
  readonly limit: number;
}

/** Returns stable data for caller-owned translated validation copy. */
export function findDimensionViolation(
  dimensions: ImageDimensions,
  bounds: DimensionBounds,
): DimensionViolation | null {
  const checks = [
    ["width", "min", dimensions.width, bounds.minWidth],
    ["height", "min", dimensions.height, bounds.minHeight],
    ["width", "max", dimensions.width, bounds.maxWidth],
    ["height", "max", dimensions.height, bounds.maxHeight],
  ] as const;

  for (const [axis, bound, actual, rawLimit] of checks) {
    const limit = positiveOrUndefined(rawLimit);
    if (limit === undefined) continue;
    if (
      (bound === "min" && actual < limit) ||
      (bound === "max" && actual > limit)
    )
      return { axis, bound, actual, limit };
  }
  return null;
}
