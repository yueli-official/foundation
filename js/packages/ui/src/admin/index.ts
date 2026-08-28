export { default as AdminPage } from "./components/AdminPage.vue";
export { default as ManagePage } from "./components/ManagePage.vue";
export { default as AdminConsoleLayout } from "./components/AdminConsoleLayout.vue";
export { default as AdminShell } from "./components/AdminShell.vue";
export { default as EditorInspector } from "./components/EditorInspector.vue";
export { default as AdminIconPicker } from "./components/AdminIconPicker.vue";
export { default as AdminRowActions } from "./components/AdminRowActions.vue";
export {
  ADMIN_TABLER_ICON_OPTIONS,
  filterAdminIconOptions,
} from "./icons";
export { default as PageHeader } from "../dashboard/components/PageHeader.vue";
export { default as TabbedSurface } from "../dashboard/components/TabbedSurface.vue";
export {
  createAdminNavigationSearchItems,
  normalizeAdminNavigation,
} from "./navigation";
export type {
  AdminNavigationChildItem,
  AdminIconOption,
  AdminNavigationItem,
  AdminRowActionItem,
  AdminSearchGroup,
  AdminSearchItem,
  AdminShellMessages,
  AdminShellUi,
} from "./types";
export type { AdminNavigationSearchOptions } from "./navigation";
