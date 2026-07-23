declare module "#imports" {
  import type { ComputedRef } from "vue";

  export function useHead(
    input: ComputedRef<Readonly<Record<string, unknown>>>,
  ): void;
}
