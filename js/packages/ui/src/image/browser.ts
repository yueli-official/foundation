import {
  calculateContainedImageSize,
  evaluateImageOptimization,
  optimizedImageFilename,
  type ImageDimensions,
  type ImageOptimizationOptions,
} from "./index";

export type ImageOptimizationResultReason =
  | "optimized"
  | "disabled"
  | "unsupported-type"
  | "animated-image"
  | "already-efficient"
  | "browser-unavailable"
  | "decode-failed"
  | "invalid-dimensions"
  | "canvas-unavailable"
  | "encode-failed"
  | "larger-output";

export interface ImageOptimizationResult {
  readonly file: File;
  readonly optimized: boolean;
  readonly reason: ImageOptimizationResultReason;
  readonly source?: ImageDimensions;
  readonly output?: ImageDimensions;
}

export interface DecodedImage {
  readonly width: number;
  readonly height: number;
  draw(context: CanvasRenderingContext2D, width: number, height: number): void;
  dispose(): void;
}

export interface ImageOptimizationRuntime {
  decode(file: File): Promise<DecodedImage>;
  createCanvas(width: number, height: number): HTMLCanvasElement;
  createFile(blob: Blob, name: string, options: FilePropertyBag): File;
  now(): number;
}

export function createBrowserImageOptimizationRuntime(): ImageOptimizationRuntime | null {
  if (
    typeof document === "undefined" ||
    typeof Image === "undefined" ||
    typeof URL === "undefined" ||
    typeof File === "undefined"
  )
    return null;

  return {
    async decode(file) {
      const objectUrl = URL.createObjectURL(file);
      const image = new Image();
      image.decoding = "async";
      image.src = objectUrl;
      try {
        await image.decode();
      } catch (error) {
        URL.revokeObjectURL(objectUrl);
        throw error;
      }
      return {
        width: image.naturalWidth,
        height: image.naturalHeight,
        draw: (context, width, height) =>
          context.drawImage(image, 0, 0, width, height),
        dispose: () => URL.revokeObjectURL(objectUrl),
      };
    },
    createCanvas(width, height) {
      const canvas = document.createElement("canvas");
      canvas.width = width;
      canvas.height = height;
      return canvas;
    },
    createFile: (blob, name, options) => new File([blob], name, options),
    now: () => Date.now(),
  };
}

function originalResult(
  file: File,
  reason: ImageOptimizationResultReason,
  source?: ImageDimensions,
  output?: ImageDimensions,
): ImageOptimizationResult {
  return { file, optimized: false, reason, source, output };
}

export async function optimizeImage(
  file: File,
  options: ImageOptimizationOptions = {},
  runtime: ImageOptimizationRuntime | null = createBrowserImageOptimizationRuntime(),
): Promise<ImageOptimizationResult> {
  const decision = evaluateImageOptimization(file, options);
  if (!decision.optimize) return originalResult(file, decision.reason);
  if (!runtime) return originalResult(file, "browser-unavailable");

  let decoded: DecodedImage;
  try {
    decoded = await runtime.decode(file);
  } catch {
    return originalResult(file, "decode-failed");
  }

  const source = { width: decoded.width, height: decoded.height };
  try {
    const output = calculateContainedImageSize(source, decision.options);
    if (!output) return originalResult(file, "invalid-dimensions", source);

    const canvas = runtime.createCanvas(output.width, output.height);
    const context = canvas.getContext("2d");
    if (!context)
      return originalResult(file, "canvas-unavailable", source, output);
    decoded.draw(context, output.width, output.height);

    let blob: Blob | null;
    try {
      blob = await new Promise<Blob | null>((resolve) =>
        canvas.toBlob(
          resolve,
          decision.options.outputType,
          decision.options.quality,
        ),
      );
    } catch {
      return originalResult(file, "encode-failed", source, output);
    }
    if (!blob) return originalResult(file, "encode-failed", source, output);
    if (decision.options.discardLarger && blob.size >= file.size)
      return originalResult(file, "larger-output", source, output);

    return {
      file: runtime.createFile(
        blob,
        optimizedImageFilename(file.name, decision.options.outputType),
        {
          type: decision.options.outputType,
          lastModified: runtime.now(),
        },
      ),
      optimized: true,
      reason: "optimized",
      source,
      output,
    };
  } finally {
    decoded.dispose();
  }
}

/** Compatibility convenience for upload pipelines that only need the File. */
export async function optimizeImageFile(
  file: File,
  options: ImageOptimizationOptions = {},
  runtime?: ImageOptimizationRuntime | null,
) {
  return (await optimizeImage(file, options, runtime)).file;
}
