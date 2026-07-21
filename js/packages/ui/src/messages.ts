export type MessageParameter =
  string | number | boolean | null | readonly MessageParameter[];

export interface MessageReference<
  TKey extends string = string,
  TParameters extends Readonly<Record<string, MessageParameter>> = Readonly<
    Record<string, MessageParameter>
  >,
> {
  readonly key: TKey;
  readonly params?: TParameters;
}

export type MessageResolver = (message: MessageReference) => string | undefined;
