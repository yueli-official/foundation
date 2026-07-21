export default defineEventHandler((event) => {
  setResponseHeader(
    event,
    "set-cookie",
    "downstream=must-not-escape; HttpOnly",
  );
  setResponseHeader(event, "x-internal-target", "must-not-escape");
  return {
    acceptLanguage: getHeader(event, "accept-language") ?? null,
    authorization: getHeader(event, "authorization") ?? null,
    cookie: getHeader(event, "cookie") ?? null,
    source: getQuery(event).source ?? null,
  };
});
