import { setHeader, type H3Event } from "h3";

import { assertDiscoveryProjection } from "./projection";
import type { DiscoveryProjection } from "./types";

export function applyDiscoveryHeaders(
  event: H3Event,
  value: DiscoveryProjection,
): void {
  assertDiscoveryProjection(value);
  if (value.headers.link !== undefined && value.headers.link.length > 0) {
    setHeader(event, "Link", [...value.headers.link]);
  }
  if (value.headers.xRobotsTag !== undefined) {
    setHeader(event, "X-Robots-Tag", value.headers.xRobotsTag);
  }
}
