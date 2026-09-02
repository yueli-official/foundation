export type FeedbackTone = "neutral" | "success" | "info" | "warning" | "error";

export interface FeedbackNoticeInput {
  readonly id?: string | number;
  readonly title?: string;
  readonly description?: string;
  readonly tone?: FeedbackTone;
  readonly duration?: number;
  readonly foreground?: boolean;
  readonly close?: boolean;
  readonly icon?: string;
}

export interface NormalizedFeedbackNotice extends FeedbackNoticeInput {
  readonly id: string | number;
  readonly tone: FeedbackTone;
  readonly duration: number;
  readonly foreground: boolean;
  readonly close: boolean;
}

export interface NuxtToastInput {
  readonly id?: string | number;
  readonly title?: string;
  readonly description?: string;
  readonly color?: string;
  readonly duration?: number;
  readonly type?: "foreground" | "background";
  readonly close?: boolean;
  readonly icon?: string;
  readonly [key: string]: unknown;
}

export interface NormalizedNuxtToastInput extends NuxtToastInput {
  readonly id: string | number;
  readonly color: FeedbackTone;
  readonly duration: number;
  readonly type: "foreground" | "background";
  readonly close: boolean;
}

const feedbackTones = new Set<FeedbackTone>([
  "neutral",
  "success",
  "info",
  "warning",
  "error",
]);

function stableNoticeId(input: FeedbackNoticeInput): string {
  const source = `${input.tone ?? "neutral"}:${input.title ?? ""}:${input.description ?? ""}`;
  let hash = 2_166_136_261;
  for (let index = 0; index < source.length; index += 1) {
    hash ^= source.charCodeAt(index);
    hash = Math.imul(hash, 16_777_619);
  }
  return `y-feedback-${(hash >>> 0).toString(36)}`;
}

export function normalizeFeedbackNotice(
  input: FeedbackNoticeInput,
): NormalizedFeedbackNotice {
  const tone = input.tone ?? "neutral";
  const needsAttention = tone === "error" || tone === "warning";
  return {
    ...input,
    id: input.id ?? stableNoticeId(input),
    tone,
    close: input.close ?? true,
    duration:
      input.duration ??
      (tone === "error" ? 6_500 : needsAttention ? 5_500 : 4_500),
    foreground: input.foreground ?? needsAttention,
    icon:
      input.icon ??
      (tone === "error"
        ? "i-tabler-alert-circle"
        : tone === "warning"
          ? "i-tabler-alert-triangle"
          : tone === "success"
            ? "i-tabler-circle-check"
            : tone === "info"
              ? "i-tabler-info-circle"
              : undefined),
  };
}

export function createFeedbackNotifier<TNative>(
  adapter: {
    add(input: TNative): unknown;
  },
  map: (notice: NormalizedFeedbackNotice) => TNative,
) {
  return {
    add(input: FeedbackNoticeInput) {
      return adapter.add(map(normalizeFeedbackNotice(input)));
    },
  };
}

export function normalizeNuxtToastInput(
  input: NuxtToastInput,
): NormalizedNuxtToastInput {
  const notice = normalizeFeedbackNotice({
    id: input.id,
    title: input.title,
    description: input.description,
    tone: feedbackTones.has(input.color as FeedbackTone)
      ? (input.color as FeedbackTone)
      : "neutral",
    duration: input.duration,
    foreground:
      input.type === undefined ? undefined : input.type === "foreground",
    close: input.close,
    icon: input.icon,
  });
  return {
    ...input,
    id: notice.id,
    color: notice.tone,
    duration: notice.duration,
    type: notice.foreground ? "foreground" : "background",
    close: notice.close,
    icon: notice.icon,
  };
}

/** Adapts the shared feedback contract to Nuxt UI's native toast input. */
export function createNuxtToastNotifier<TNative>(toast: {
  add(input: TNative): unknown;
}) {
  return {
    add(input: NuxtToastInput) {
      return toast.add(normalizeNuxtToastInput(input) as TNative);
    },
  };
}
