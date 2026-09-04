import { getApiFailure, type ApiFailure, type ProblemParams } from "./failure";

export interface FailureText {
  readonly message: string;
  readonly recovery?: string;
}

export type FailureTextResolver = (
  code: string,
  params: ProblemParams,
) => FailureText | undefined;

export interface ResolveFailureOptions {
  /** Safe, localized action-specific text such as "创建文档集失败". */
  readonly fallback: string;
  readonly resolveText: FailureTextResolver;
  /** Maps RFC 6901 violation pointers to product form field keys. */
  readonly fields?: Readonly<Record<string, string>>;
}

export interface FailureFeedback {
  readonly message: string;
  readonly recovery?: string;
  readonly fieldErrors: Readonly<Record<string, readonly string[]>>;
  readonly summary: readonly string[];
  readonly technical: {
    readonly code: string;
    readonly traceId?: string;
  };
  readonly failure?: ApiFailure;
}

export function resolveFailureFeedback(
  error: unknown,
  options: ResolveFailureOptions,
): FailureFeedback {
  const failure = getApiFailure(error);
  if (!failure) {
    return {
      message: options.fallback,
      fieldErrors: {},
      summary: [],
      technical: { code: "foundation.unknown" },
    };
  }

  const text = options.resolveText(failure.code, failure.params ?? {});
  const fieldErrors: Record<string, string[]> = {};
  const summary: string[] = [];
  if (failure.kind === "remote") {
    for (const violation of failure.violations) {
      const violationText =
        options.resolveText(violation.code, violation.params)?.message ??
        options.fallback;
      const field = fieldForPointer(violation.pointer, options.fields);
      if (field) {
        (fieldErrors[field] ??= []).push(violationText);
      } else {
        summary.push(violationText);
      }
    }
  }

  return {
    message: text?.message ?? options.fallback,
    ...(text?.recovery ? { recovery: text.recovery } : {}),
    fieldErrors,
    summary,
    technical: {
      code: failure.code,
      ...(failure.traceId ? { traceId: failure.traceId } : {}),
    },
    failure,
  };
}

function fieldForPointer(
  pointer: string,
  fields: ResolveFailureOptions["fields"],
): string | undefined {
  if (!fields || pointer === "") {
    return undefined;
  }
  if (fields[pointer]) {
    return fields[pointer];
  }
  const first = pointer.split("/")[1];
  if (!first) {
    return undefined;
  }
  const decoded = first.replaceAll("~1", "/").replaceAll("~0", "~");
  return fields[`/${first}`] ?? fields[decoded];
}
