import { describe, expect, it, vi } from "vitest";
import {
  createFeedbackNotifier,
  normalizeFeedbackNotice,
} from "../src/feedback/notice";

describe("feedback notices", () => {
  it("normalizes attention states without accepting backend-specific fields", () => {
    const first = normalizeFeedbackNotice({
      title: "Save failed",
      description: "Try again",
      tone: "error",
    });
    const second = normalizeFeedbackNotice({
      title: "Save failed",
      description: "Try again",
      tone: "error",
    });

    expect(first).toMatchObject({
      id: second.id,
      tone: "error",
      close: true,
      duration: 6_500,
      foreground: true,
      icon: "i-tabler-alert-circle",
    });
  });

  it("maps the neutral contract at the caller-owned toast seam", () => {
    const add = vi.fn();
    const notifier = createFeedbackNotifier({ add }, (notice) => ({
      ...notice,
      color: notice.tone,
      type: "foreground",
    }));

    notifier.add({ title: "Saved", tone: "success" });
    expect(add).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "Saved",
        color: "success",
        type: "foreground",
      }),
    );
  });
});
