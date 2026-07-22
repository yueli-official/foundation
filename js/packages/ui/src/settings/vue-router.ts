import { onBeforeRouteLeave } from "vue-router";

export interface SettingsLeaveGuardOptions {
  readonly isDirty: () => boolean;
  readonly confirm: () => boolean | Promise<boolean>;
}

export function useSettingsLeaveGuard(
  options: SettingsLeaveGuardOptions,
): void {
  onBeforeRouteLeave(async () => {
    if (!options.isDirty()) return true;
    return options.confirm();
  });
}
