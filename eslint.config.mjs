import { createConfigForNuxt } from "@nuxt/eslint-config";

export default createConfigForNuxt({
  features: { stylistic: false },
}).append({
  ignores: [
    "**/.nuxt/**",
    "**/.output/**",
    "**/node_modules/**",
    "**/test-results/**",
    "**/playwright-report/**",
  ],
  rules: {
    "vue/multi-word-component-names": "off",
    "vue/attributes-order": "off",
    "vue/first-attribute-linebreak": "off",
    "vue/html-self-closing": "off",
    "vue/require-default-prop": "off",
  },
});
