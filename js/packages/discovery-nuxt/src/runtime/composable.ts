import { computed, type MaybeRefOrGetter, toValue } from "vue";
import { useHead } from "#imports";

import { toDiscoveryHead } from "../projection";
import type { DiscoveryProjection } from "../types";

export function useDiscoveryPage(
  projection: MaybeRefOrGetter<DiscoveryProjection | null | undefined>,
): void {
  useHead(
    computed(() => {
      const value = toValue(projection);
      return value === null || value === undefined
        ? {}
        : toDiscoveryHead(value);
    }),
  );
}
