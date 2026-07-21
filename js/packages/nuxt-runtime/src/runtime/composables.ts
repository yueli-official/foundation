import type { ApiClient } from "@yueli/http-runtime";
import { useNuxtApp } from "#app";

export function useApi(target?: string): ApiClient {
  return useNuxtApp().$yueliApi(target);
}
