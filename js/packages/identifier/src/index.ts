import { validate as validateUUID, v5, v7, version as uuidVersion } from "uuid";

export const CompactURLV1 = "compact-url-v1" as const;
export const ShortLocatorV1 = "short-locator-v1" as const;
export const HumanCodeV1 = "human-code-v1" as const;
export const OpaquePublicV1 = "opaque-public-v1" as const;

export type KeyProfile =
  | typeof CompactURLV1
  | typeof ShortLocatorV1
  | typeof HumanCodeV1
  | typeof OpaquePublicV1;

export const Claimed = "claimed" as const;
export const Collision = "collision" as const;
export type ClaimResult = typeof Claimed | typeof Collision;
export type Claim = (candidate: string) => ClaimResult | Promise<ClaimResult>;

export const MaxAllocationAttempts = 8;

export type IdentifierErrorCode =
  | "invalid_uuid"
  | "invalid_key"
  | "invalid_profile"
  | "entropy_unavailable"
  | "collision_exhausted"
  | "allocation_aborted"
  | "invalid_claim_result";

export class IdentifierError extends Error {
  readonly code: IdentifierErrorCode;

  constructor(code: IdentifierErrorCode, message: string, options?: ErrorOptions) {
    super(message, options);
    this.name = "IdentifierError";
    this.code = code;
  }
}

export type KeyProfileDefinition = Readonly<{
  id: KeyProfile;
  alphabet: string;
  length: number;
  case: "sensitive" | "canonical-uppercase";
  purpose: "public-locator";
  allocation: "atomic-unique-claim";
}>;

const definitions: Readonly<Record<KeyProfile, KeyProfileDefinition>> =
  Object.freeze({
    [CompactURLV1]: Object.freeze({
      id: CompactURLV1,
      alphabet: "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz",
      length: 8,
      case: "sensitive",
      purpose: "public-locator",
      allocation: "atomic-unique-claim",
    }),
    [ShortLocatorV1]: Object.freeze({
      id: ShortLocatorV1,
      alphabet: "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz",
      length: 6,
      case: "sensitive",
      purpose: "public-locator",
      allocation: "atomic-unique-claim",
    }),
    [HumanCodeV1]: Object.freeze({
      id: HumanCodeV1,
      alphabet: "0123456789ABCDEFGHJKMNPQRSTVWXYZ",
      length: 10,
      case: "canonical-uppercase",
      purpose: "public-locator",
      allocation: "atomic-unique-claim",
    }),
    [OpaquePublicV1]: Object.freeze({
      id: OpaquePublicV1,
      alphabet: "0123456789ABCDEFGHJKMNPQRSTVWXYZ",
      length: 16,
      case: "canonical-uppercase",
      purpose: "public-locator",
      allocation: "atomic-unique-claim",
    }),
  });

const canonicalUUID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

export function newUUID(): string {
  try {
    return v7();
  } catch (cause) {
    throw new IdentifierError(
      "entropy_unavailable",
      "identifier: cryptographic entropy unavailable",
      { cause },
    );
  }
}

// keyProfile returns immutable wire metadata so consumers can validate a
// profile without copying alphabets or lengths into product code.
export function keyProfile(profile: KeyProfile): KeyProfileDefinition {
  return profileDefinition(profile);
}

export function parseUUID(text: string): string {
  if (!canonicalUUID.test(text) || !validateUUID(text)) {
    throw new IdentifierError(
      "invalid_uuid",
      "identifier: invalid canonical UUID",
    );
  }
  return text;
}

export function UUIDVersion(text: string): number {
  return uuidVersion(parseUUID(text));
}

export function deriveUUID(
  namespace: string,
  canonicalName: Uint8Array,
): string {
  return v5(canonicalName, parseUUID(namespace));
}

export function newKey(profile: KeyProfile): string {
  const definition = profileDefinition(profile);
  const limit = 256 - (256 % definition.alphabet.length);
  const output: string[] = [];
  const samples = new Uint8Array(
    new ArrayBuffer(Math.max(16, definition.length * 2)),
  );
  while (output.length < definition.length) {
    fillRandom(samples);
    for (const sample of samples) {
      if (sample >= limit) continue;
      output.push(definition.alphabet[sample % definition.alphabet.length]!);
      if (output.length === definition.length) break;
    }
  }
  return output.join("");
}

export function parseKey(profile: KeyProfile, text: string): string {
  const definition = profileDefinition(profile);
  if (text.length !== definition.length) {
    throw new IdentifierError("invalid_key", "identifier: invalid public key");
  }
  for (const character of text) {
    if (!definition.alphabet.includes(character)) {
      throw new IdentifierError("invalid_key", "identifier: invalid public key");
    }
  }
  return text;
}

export async function allocateKey(
  profile: KeyProfile,
  claim: Claim,
  options: { signal?: AbortSignal } = {},
): Promise<string> {
  for (let attempt = 0; attempt < MaxAllocationAttempts; attempt += 1) {
    if (options.signal?.aborted) {
      throw new IdentifierError(
        "allocation_aborted",
        "identifier: allocation aborted",
        { cause: options.signal.reason },
      );
    }
    const candidate = newKey(profile);
    const result = await claim(candidate);
    if (result === Claimed) return candidate;
    if (result !== Collision) {
      throw new IdentifierError(
        "invalid_claim_result",
        "identifier: claim returned an invalid result",
      );
    }
  }
  throw new IdentifierError(
    "collision_exhausted",
    "identifier: public key collision attempts exhausted",
  );
}

function profileDefinition(profile: KeyProfile): KeyProfileDefinition {
  const definition = definitions[profile];
  if (!definition) {
    throw new IdentifierError(
      "invalid_profile",
      "identifier: invalid key profile",
    );
  }
  return definition;
}

function fillRandom(target: Uint8Array<ArrayBuffer>): void {
  try {
    if (!globalThis.crypto?.getRandomValues) throw new Error("Web Crypto unavailable");
    globalThis.crypto.getRandomValues(target);
  } catch (cause) {
    throw new IdentifierError(
      "entropy_unavailable",
      "identifier: cryptographic entropy unavailable",
      { cause },
    );
  }
}
