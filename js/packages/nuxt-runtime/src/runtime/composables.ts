import type { ApiClient } from "@yueli/http-runtime";
import { useNuxtApp } from "#app";

export { createBrowserUUID } from "./identifiers";

export function useApi(target?: string): ApiClient {
  return useNuxtApp().$yueliApi(target);
}
