import type { ApiClient } from "@yueli/http-runtime";

declare module "#app" {
  interface NuxtApp {
    $yueliApi(target?: string): ApiClient;
  }
}

declare module "vue" {
  interface ComponentCustomProperties {
    $yueliApi(target?: string): ApiClient;
  }
}

export {};
