export default defineNuxtConfig({
  devtools: { enabled: false },
  modules: ["@yueli/nuxt-runtime"],
  runtimeConfig: {
    conformanceDownstreamOrigin: "",
  },
  ssr: true,
  yueliRuntime: {
    defaultTarget: "content",
    targets: {
      content: {
        path: "/api/bff/content",
        ssr: {
          cookies: ["session"],
          headers: ["accept-language"],
        },
      },
      identity: { path: "/api/bff/identity" },
      proxy: {
        path: "/api/bff/proxy",
        ssr: {
          cookies: ["session"],
          headers: ["accept-language"],
        },
      },
    },
  },
});
