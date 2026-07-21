export default defineNuxtConfig({
  modules: ["@nuxt/ui", "@yueli/ui"],
  css: ["~/assets/css/main.css"],
  devtools: { enabled: false },
  ssr: true,
  app: {
    head: {
      htmlAttrs: { lang: "zh-CN" },
      title: "Yueli Foundation UI Lab",
    },
  },
  fonts: {
    providers: {
      google: false,
      googleicons: false,
      bunny: false,
      fontshare: false,
      fontsource: false,
    },
  },
});
