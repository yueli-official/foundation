<script setup lang="ts">
import { getApiFailure } from "@yueli/http-runtime";
import { useApi } from "@yueli/nuxt-runtime/runtime";

const api = useApi();
const { data, error } = await useAsyncData("articles", () =>
  api.request<{ items: Array<{ id: string; title: string }> }>("/articles"),
);
const failure = computed(() => getApiFailure(error.value));
const { error: conflictError } = await useAsyncData("conflict", () =>
  api.request("/conflict"),
);
const conflictFailure = computed(() => getApiFailure(conflictError.value));
const { data: requestContext } = await useAsyncData("request-context", () =>
  api.request<{
    cookie: string | null;
    acceptLanguage: string | null;
    authorization: string | null;
    forwardedFor: string | null;
  }>("/context"),
);
const proxyApi = useApi("proxy");
const { data: downstreamContext } = await useAsyncData(
  "downstream-context",
  () =>
    proxyApi.request<{
      acceptLanguage: string | null;
      authorization: string | null;
      cookie: string | null;
      source: string | null;
    }>("/echo", { query: { source: "ssr" } }),
);
const mounted = ref(false);
onMounted(() => {
  mounted.value = true;
});
</script>

<template>
  <main>
    <h1>HTTP runtime conformance</h1>
    <output v-if="data" data-testid="result">{{ data.items[0]?.title }}</output>
    <output v-else-if="failure" data-testid="failure">{{
      failure.code
    }}</output>
    <output v-if="mounted && conflictFailure" data-testid="hydrated-failure">{{
      conflictFailure.code
    }}</output>
    <output v-if="requestContext" data-testid="request-context">{{
      JSON.stringify(requestContext)
    }}</output>
    <output v-if="downstreamContext" data-testid="downstream-context">{{
      JSON.stringify(downstreamContext)
    }}</output>
  </main>
</template>
