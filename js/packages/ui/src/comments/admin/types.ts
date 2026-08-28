import type {
  CollectionControl,
  CollectionControlValue,
  CollectionPanelState,
} from "../../collection/panel";
import type { AdminRowActionItem } from "../../admin/types";
import type { RouteLocationRaw } from "vue-router";

export type CommentModerationColor =
  | "primary"
  | "neutral"
  | "success"
  | "info"
  | "warning"
  | "error";
export type CommentModerationLifecycle =
  | "all"
  | "pending"
  | "approved"
  | "spam"
  | "trash";

export interface CommentModerationStatus {
  readonly label: string;
  readonly color: CommentModerationColor;
  readonly icon?: string;
}

export interface CommentModerationSource {
  readonly label: string;
  readonly to?: RouteLocationRaw;
  readonly icon: string;
}

export interface CommentModerationItem {
  readonly id: string;
  readonly content: string;
  readonly createdAt: string;
  readonly authorName: string;
  readonly avatarUrl?: string;
  readonly authorEmail?: string;
  readonly anonymous?: boolean;
  readonly reply?: boolean;
  readonly approve?: boolean;
  readonly approving?: boolean;
  readonly actions?:
    | readonly AdminRowActionItem[]
    | readonly (readonly AdminRowActionItem[])[];
  readonly status?: CommentModerationStatus;
  readonly source: CommentModerationSource;
}

export interface CommentModerationSelection {
  readonly enabled: boolean;
  readonly count: number;
  readonly pageSelected: boolean;
  readonly pageIndeterminate: boolean;
  readonly isSelected: (id: string) => boolean;
}

export interface CommentModerationCollectionModel {
  readonly search: string;
  readonly searchPlaceholder: string;
  readonly items: readonly CommentModerationItem[];
  readonly state: CollectionPanelState;
  readonly errorMessage?: string;
  readonly total: number;
  readonly page: number;
  readonly pageSize: number;
  readonly pageSizes?: readonly number[];
  readonly activeFilterCount?: number;
  readonly controls?: readonly CollectionControl[];
  readonly sortOrder: "asc" | "desc";
  readonly lifecycle: CommentModerationLifecycle;
  readonly emptyingTrash?: boolean;
  readonly selection?: CommentModerationSelection;
}

export interface CommentModerationCollectionActions {
  readonly updateSearch: (value: string) => void;
  readonly search: (value: string) => void;
  readonly controlChange: (id: string, value: CollectionControlValue) => void;
  readonly clearFilters: () => void;
  readonly retry: () => void | Promise<void>;
  readonly sort: () => void;
  readonly lifecycleChange: (value: CommentModerationLifecycle) => void;
  readonly emptyTrash?: () => boolean | void | Promise<boolean | void>;
  readonly approve?: (id: string) => void | Promise<void>;
  readonly pageChange: (page: number) => void;
  readonly pageSizeChange: (pageSize: number) => void;
  readonly togglePage?: (selected: boolean) => void;
  readonly toggleItem?: (id: string, selected: boolean) => void;
  readonly clearSelection?: () => void;
}
