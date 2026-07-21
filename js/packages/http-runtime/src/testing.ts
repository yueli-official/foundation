import type { Transport, TransportRequest } from "./index";

export interface MemoryTransport extends Transport {
  readonly requests: readonly TransportRequest[];
}

export function createMemoryTransport(
  handler: (request: TransportRequest) => Response | Promise<Response>,
): MemoryTransport {
  const requests: TransportRequest[] = [];

  return {
    requests,
    async send(request) {
      requests.push(request);
      return handler(request);
    },
  };
}
