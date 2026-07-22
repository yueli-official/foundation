export type RemoteSelectValue = string | number;

export interface RemoteSelectOption<
  TValue extends RemoteSelectValue = RemoteSelectValue,
> {
  readonly value: TValue;
  readonly label: string;
  readonly description?: string;
  readonly icon?: string;
  readonly disabled?: boolean;
}

export interface RemoteSelectLoadRequest {
  readonly query: string;
  readonly signal: AbortSignal;
}

export interface RemoteSelectLoadResult<
  TValue extends RemoteSelectValue = RemoteSelectValue,
> {
  readonly items: readonly RemoteSelectOption<TValue>[];
}

export type RemoteSelectLoader<
  TValue extends RemoteSelectValue = RemoteSelectValue,
> = (
  request: RemoteSelectLoadRequest,
) => Promise<RemoteSelectLoadResult<TValue>>;

export interface RemoteSelectMessages {
  readonly placeholder: string;
  readonly searchPlaceholder: string;
  readonly empty: string;
  readonly error: string;
  readonly retry: string;
  readonly minimumQuery: (count: number) => string;
}
