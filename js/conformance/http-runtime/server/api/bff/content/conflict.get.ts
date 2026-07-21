export default defineEventHandler((event) => {
  setResponseStatus(event, 409);
  setResponseHeader(event, "content-type", "application/problem+json");
  setResponseHeader(event, "x-trace-id", "trace-hydration-conflict");
  return {
    type: "https://docs.yueli.dev/problems/blog.slug_taken",
    status: 409,
    code: "blog.slug_taken",
    params: { slug: "hello" },
    traceId: "trace-hydration-conflict",
  };
});
