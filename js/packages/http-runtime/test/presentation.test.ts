import { describe, expect, test } from "vitest";

import {
  resolveFailureFeedback,
  type ApiFailure,
  type ProblemParams,
} from "../src/index";

const messages: Record<string, (params: ProblemParams) => string> = {
  "docs.invalid_input": () => "请检查输入内容。",
  "validation.required": () => "此项为必填项。",
};

const resolveText = (code: string, params: ProblemParams) => {
  const message = messages[code]?.(params);
  return message ? { message } : undefined;
};

describe("resolveFailureFeedback", () => {
  test("projects violations to fields without exposing machine codes as copy", () => {
    const failure: ApiFailure = {
      kind: "remote",
      status: 400,
      code: "docs.invalid_input",
      params: {},
      violations: [
        { pointer: "/title", code: "validation.required", params: {} },
        {
          pointer: "/documents/2/slug",
          code: "validation.required",
          params: {},
        },
      ],
      traceId: "trace-42",
      reauth: "not-attempted",
    };
    const feedback = resolveFailureFeedback(
      { failure },
      {
        fallback: "创建文档集失败。",
        resolveText,
        fields: { title: "title" },
      },
    );

    expect(feedback.message).toBe("请检查输入内容。");
    expect(feedback.fieldErrors).toEqual({ title: ["此项为必填项。"] });
    expect(feedback.summary).toEqual(["此项为必填项。"]);
    expect(feedback.technical).toEqual({
      code: "docs.invalid_input",
      traceId: "trace-42",
    });
    expect(feedback.message).not.toContain("docs.invalid_input");
  });

  test("uses an action-specific safe fallback for unknown and unstructured errors", () => {
    const fallback = "上传失败，请稍后重试。";
    expect(
      resolveFailureFeedback(new Error("private SQL error"), {
        fallback,
        resolveText,
      }),
    ).toMatchObject({
      message: fallback,
      technical: { code: "foundation.unknown" },
    });
    const local: ApiFailure = {
      kind: "timeout",
      code: "foundation.request.timeout",
      reauth: "not-attempted",
    };
    expect(
      resolveFailureFeedback(local, { fallback, resolveText }),
    ).toMatchObject({
      message: fallback,
      technical: { code: "foundation.request.timeout" },
    });
  });
});
