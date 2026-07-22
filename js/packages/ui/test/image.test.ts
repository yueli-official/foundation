// @vitest-environment happy-dom
import { describe, expect, it, vi } from "vitest";
import {
  calculateContainedImageSize,
  createCropperJsConfig,
  createCropperOutputOptions,
  cropFilename,
  evaluateImageOptimization,
  findDimensionViolation,
  normalizeImageOptimizationOptions,
  optimizedImageFilename,
} from "../src/image/index";
import {
  optimizeImage,
  type ImageOptimizationRuntime,
} from "../src/image/browser";

describe("image policy", () => {
  it("returns stable eligibility reasons and excludes active SVG content", () => {
    expect(
      evaluateImageOptimization({
        name: "cover.svg",
        type: "image/svg+xml",
        size: 1,
      }),
    ).toMatchObject({ optimize: false, reason: "unsupported-type" });
    expect(
      evaluateImageOptimization({
        name: "cover.gif",
        type: "image/gif",
        size: 1,
      }),
    ).toMatchObject({ optimize: false, reason: "animated-image" });
    expect(
      evaluateImageOptimization({
        name: "cover.png",
        type: "image/png",
        size: 2_000_000,
      }),
    ).toMatchObject({ optimize: true, reason: "eligible" });
  });

  it("clamps unsafe options and constrains side length plus pixel area", () => {
    expect(
      normalizeImageOptimizationOptions({
        maxSide: 20,
        quality: 4,
        maxPixels: Infinity,
      }),
    ).toMatchObject({ maxSide: 320, quality: 1, maxPixels: 16_777_216 });
    expect(
      calculateContainedImageSize(
        { width: 12_000, height: 8_000 },
        { maxSide: 4096, maxPixels: 16_000_000 },
      ),
    ).toEqual({ width: 4096, height: 2731 });
    expect(
      calculateContainedImageSize(
        { width: Number.NaN, height: 1 },
        { maxSide: 1920, maxPixels: 16_000_000 },
      ),
    ).toBeNull();
  });

  it("creates stable filenames", () => {
    expect(optimizedImageFilename("cover.large.png", "image/webp")).toBe(
      "cover.large.webp",
    );
    expect(cropFilename("cover.large.png", "image/jpeg")).toBe(
      "cover.large-crop.jpg",
    );
  });
});

describe("crop policy", () => {
  it("builds dependency-compatible fixed, circle and free crop configs", () => {
    expect(
      createCropperJsConfig({
        mode: "fixed-size",
        aspectRatio: 3 / 2,
        outputWidth: 1200,
        outputHeight: 800,
        minCropWidth: 600,
      }).options,
    ).toMatchObject({ aspectRatio: 1.5, minCropBoxWidth: 600 });
    expect(createCropperJsConfig({ shape: "circle" }).options.aspectRatio).toBe(
      1,
    );
    expect(
      createCropperJsConfig({ mode: "free" }).options.aspectRatio,
    ).toBeUndefined();
  });

  it("normalizes canvas output and returns caller-translatable violations", () => {
    expect(
      createCropperOutputOptions({
        outputWidth: 512,
        outputHeight: 512,
        outputType: "image/jpeg",
        shape: "circle",
      }),
    ).toEqual({
      width: 512,
      height: 512,
      rounded: true,
      fillColor: "#fff",
      imageSmoothingQuality: "high",
    });
    expect(
      findDimensionViolation(
        { width: 320, height: 200 },
        { minWidth: 600, maxHeight: 1000 },
      ),
    ).toEqual({ axis: "width", bound: "min", actual: 320, limit: 600 });
  });
});

describe("browser image optimization", () => {
  function runtime(blob: Blob) {
    const dispose = vi.fn();
    const draw = vi.fn();
    const context = {} as CanvasRenderingContext2D;
    const canvas = {
      getContext: vi.fn(() => context),
      toBlob: vi.fn((callback: BlobCallback) => callback(blob)),
    } as unknown as HTMLCanvasElement;
    const value: ImageOptimizationRuntime = {
      decode: vi.fn(async () => ({ width: 2400, height: 1200, draw, dispose })),
      createCanvas: vi.fn(() => canvas),
      createFile: (nextBlob, name, options) =>
        new File([nextBlob], name, options),
      now: () => 42,
    };
    return { value, dispose, draw };
  }

  it("encodes through an injected runtime and always disposes decoded resources", async () => {
    const adapter = runtime(new Blob(["small"], { type: "image/webp" }));
    const source = new File([new Uint8Array(200)], "cover.png", {
      type: "image/png",
    });
    const result = await optimizeImage(
      source,
      { maxSide: 1200 },
      adapter.value,
    );

    expect(result).toMatchObject({
      optimized: true,
      reason: "optimized",
      source: { width: 2400, height: 1200 },
      output: { width: 1200, height: 600 },
    });
    expect(result.file.name).toBe("cover.webp");
    expect(adapter.draw).toHaveBeenCalledOnce();
    expect(adapter.dispose).toHaveBeenCalledOnce();
  });

  it("keeps the original when an encoded result is not smaller", async () => {
    const adapter = runtime(new Blob([new Uint8Array(400)]));
    const source = new File([new Uint8Array(200)], "cover.png", {
      type: "image/png",
    });
    const result = await optimizeImage(source, {}, adapter.value);

    expect(result).toMatchObject({ optimized: false, reason: "larger-output" });
    expect(result.file).toBe(source);
    expect(adapter.dispose).toHaveBeenCalledOnce();
  });
});
