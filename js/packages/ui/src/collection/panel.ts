import type { CollectionKey } from "./workflow";

export type CollectionControlValue = string | number;

export interface CollectionControlOption {
  readonly label: string;
  readonly value: CollectionControlValue;
  readonly icon?: string;
}

export interface CollectionSelectControl {
  readonly kind: "select";
  readonly id: string;
  readonly label: string;
  readonly value: CollectionControlValue;
  readonly options: readonly CollectionControlOption[];
  readonly icon?: string;
  readonly class?: string;
  readonly searchPlaceholder?: string;
}

export interface CollectionDirectionControl {
  readonly kind: "direction";
  readonly id: string;
  readonly label: string;
  readonly value: "asc" | "desc";
  readonly ascendingLabel: string;
  readonly descendingLabel: string;
}

export type CollectionControl =
  CollectionSelectControl | CollectionDirectionControl;

export type CollectionPanelState = "ready" | "loading" | "error";
export type CollectionPanelLayout = "rows" | "grid";

export interface CollectionPanelMessages {
  readonly searchPlaceholder: string;
  readonly searchAction: string;
  readonly filtersAction: string;
  readonly activeFilters: (count: number) => string;
  readonly clearFilters: string;
  readonly selectPage: string;
  readonly selectItem: (label: string) => string;
  readonly bulkRegion: string;
  readonly selected: (count: number, mode: "keys" | "query") => string;
  readonly selectAllResults: string;
  readonly clearSelection: string;
  readonly emptyTitle: string;
  readonly emptyDescription: string;
  readonly errorTitle: string;
  readonly retry: string;
  readonly showing: (first: number, last: number, total: number) => string;
  readonly pageSize: string;
  readonly pageSizeControl: string;
  readonly pageSizeOption: (size: number) => string;
}

export interface CollectionItemSlotProps<TItem> {
  readonly item: TItem;
  readonly key: CollectionKey;
  readonly selected: boolean;
  readonly toggle: () => void;
}
