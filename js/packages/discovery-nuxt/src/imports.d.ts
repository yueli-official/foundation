declare module "#imports" {
  import type { H3Event } from "h3";
  import type { ComputedRef } from "vue";

  export function useHead(
    input: ComputedRef<Readonly<Record<string, unknown>>>,
  ): void;
  export function useRuntimeConfig(
    event?: H3Event,
  ): Record<string, unknown>;
}
