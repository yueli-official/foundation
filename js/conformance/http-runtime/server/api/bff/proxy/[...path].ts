import { createBffHandler } from "@yueli/nuxt-runtime/server";

export default createBffHandler({
  mountPath: "/api/bff/proxy",
  resolveTarget: ({ event }) => ({
    origin: useRuntimeConfig(event).conformanceDownstreamOrigin,
    pathPrefix: "/api/downstream",
  }),
  credential: {
    resolve: async () => ({
      kind: "bearer",
      token: "conformance-server-token",
    }),
  },
});
