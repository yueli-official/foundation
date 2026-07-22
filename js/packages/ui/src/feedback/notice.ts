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
