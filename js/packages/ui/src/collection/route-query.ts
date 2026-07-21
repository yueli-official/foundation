export type RouteQueryValue = string | readonly string[] | null | undefined;
export type RouteQuery = Readonly<Record<string, RouteQueryValue>>;

export interface RouteQueryCodec<TQuery> {
  readonly ownedKeys: readonly string[];
  parse(query: RouteQuery): TQuery;
  serialize(query: TQuery): RouteQuery;
}

export interface CollectionRouteStringField {
  readonly kind: "string";
  readonly default: string;
  readonly maxLength?: number;
  readonly trim?: boolean;
}

export interface CollectionRouteEnumField<TValue extends string = string> {
  readonly kind: "enum";
  readonly values: readonly TValue[];
  readonly default: TValue;
}

export interface CollectionRoutePositiveIntegerField<
  TValue extends number = number,
> {
  readonly kind: "positive-integer";
  readonly values?: readonly TValue[];
  readonly default: TValue;
}

export type CollectionRouteField =
  | CollectionRouteStringField
  | CollectionRouteEnumField
  | CollectionRoutePositiveIntegerField;

type CollectionRouteFieldValue<TField extends CollectionRouteField> =
  TField extends CollectionRouteStringField
    ? string
    : TField extends CollectionRouteEnumField<infer TValue>
      ? TValue
      : TField extends CollectionRoutePositiveIntegerField
        ? number
        : never;

export type CollectionRouteQueryFromFields<
  TFields extends Readonly<Record<string, CollectionRouteField>>,
> = {
  readonly [TKey in keyof TFields]: CollectionRouteFieldValue<TFields[TKey]>;
};

function firstString(value: unknown): string {
  return Array.isArray(value) ? String(value[0] ?? "") : String(value ?? "");
}

function validateField(key: string, field: CollectionRouteField) {
  if (field.kind === "string") {
    if (
      field.maxLength !== undefined &&
      (!Number.isSafeInteger(field.maxLength) || field.maxLength < 1)
    ) {
      throw new RangeError(
        `Collection route field ${key} maxLength must be a positive safe integer.`,
      );
    }
    return;
  }
  if (field.kind === "enum") {
    if (!field.values.includes(field.default)) {
      throw new RangeError(
        `Collection route field ${key} default must be included in values.`,
      );
    }
    return;
  }
  if (!Number.isSafeInteger(field.default) || field.default < 1) {
    throw new RangeError(
      `Collection route field ${key} default must be a positive safe integer.`,
    );
  }
  if (field.values && !field.values.includes(field.default)) {
    throw new RangeError(
      `Collection route field ${key} default must be included in values.`,
    );
  }
}

function normalizeField(
  field: CollectionRouteField,
  value: unknown,
): string | number {
  if (field.kind === "string") {
    const normalized =
      field.trim === false ? firstString(value) : firstString(value).trim();
    if (!normalized) return field.default;
    return field.maxLength === undefined
      ? normalized
      : normalized.slice(0, field.maxLength);
  }
  if (field.kind === "enum") {
    const normalized = firstString(value);
    return field.values.includes(normalized) ? normalized : field.default;
  }
  const normalized = Number.parseInt(firstString(value), 10);
  if (!Number.isSafeInteger(normalized) || normalized < 1) return field.default;
  return !field.values || field.values.includes(normalized)
    ? normalized
    : field.default;
}

export function createCollectionRouteQueryCodec<
  const TFields extends Readonly<Record<string, CollectionRouteField>>,
>(fields: TFields): RouteQueryCodec<CollectionRouteQueryFromFields<TFields>> {
  const entries = Object.entries(fields);
  for (const [key, field] of entries) validateField(key, field);

  return {
    ownedKeys: Object.freeze(entries.map(([key]) => key)),
    parse(query) {
      return Object.freeze(
        Object.fromEntries(
          entries.map(([key, field]) => [
            key,
            normalizeField(field, query[key]),
          ]),
        ),
      ) as CollectionRouteQueryFromFields<TFields>;
    },
    serialize(query) {
      const serialized: Record<string, string> = {};
      for (const [key, field] of entries) {
        const value = normalizeField(field, query[key]);
        if (!Object.is(value, field.default)) serialized[key] = String(value);
      }
      return serialized;
    },
  };
}
