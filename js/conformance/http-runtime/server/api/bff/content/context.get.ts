export default defineEventHandler((event) => ({
  cookie: getHeader(event, "cookie") ?? null,
  acceptLanguage: getHeader(event, "accept-language") ?? null,
  authorization: getHeader(event, "authorization") ?? null,
  forwardedFor: getHeader(event, "x-forwarded-for") ?? null,
}));
