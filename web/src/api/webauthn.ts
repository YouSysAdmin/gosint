// Browser glue for WebAuthn (passkeys), ported from mailyard.
//
// The server speaks the standard WebAuthn JSON, where every binary
// field is a base64url string. The browser APIs want and produce
// ArrayBuffers. These helpers convert between the two so nothing else
// in the console has to know that.

function b64uToBuf(s: string): ArrayBuffer {
  const t = s.replace(/-/g, '+').replace(/_/g, '/')
  const pad = t.length % 4 ? '='.repeat(4 - (t.length % 4)) : ''
  const bin = atob(t + pad)
  const out = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i)
  return out.buffer
}

function bufToB64u(buf: ArrayBuffer): string {
  const bytes = new Uint8Array(buf)
  let bin = ''
  for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i])
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '')
}

// The server's options, before the binary fields are decoded. Typed
// loosely on purpose: this is the one place that reaches into the
// shape.
interface ServerOptions {
  publicKey: Record<string, unknown>
}

/** Whether this browser can do WebAuthn at all. */
export function supported(): boolean {
  return typeof window !== 'undefined' && !!window.PublicKeyCredential
}

/**
 * Whether a passkey can actually be used here. A browser may expose
 * the API and still have no authenticator to offer.
 */
export async function available(): Promise<boolean> {
  if (!supported()) return false
  try {
    const probe = window.PublicKeyCredential.isConditionalMediationAvailable
    if (typeof probe === 'function') return await probe()
    return true
  } catch {
    return true
  }
}

/** Runs the registration ceremony and returns the attestation JSON. */
export async function createCredential(opts: ServerOptions) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const pk = structuredClone(opts.publicKey) as Record<string, any>
  pk.challenge = b64uToBuf(pk.challenge)
  pk.user.id = b64uToBuf(pk.user.id)
  if (Array.isArray(pk.excludeCredentials)) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    pk.excludeCredentials = pk.excludeCredentials.map((c: any) => ({ ...c, id: b64uToBuf(c.id) }))
  }
  const cred = (await navigator.credentials.create({
    publicKey: pk as PublicKeyCredentialCreationOptions,
  })) as PublicKeyCredential | null
  if (!cred) throw new Error('Passkey registration was canceled')
  const res = cred.response as AuthenticatorAttestationResponse
  return {
    id: cred.id,
    rawId: bufToB64u(cred.rawId),
    type: cred.type,
    authenticatorAttachment: cred.authenticatorAttachment || undefined,
    clientExtensionResults: cred.getClientExtensionResults?.() ?? {},
    response: {
      clientDataJSON: bufToB64u(res.clientDataJSON),
      attestationObject: bufToB64u(res.attestationObject),
      transports: res.getTransports?.() ?? undefined,
    },
  }
}

/** Runs the assertion (sign-in) ceremony and returns the JSON. */
export async function getCredential(opts: ServerOptions) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const pk = structuredClone(opts.publicKey) as Record<string, any>
  pk.challenge = b64uToBuf(pk.challenge)
  if (Array.isArray(pk.allowCredentials)) {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    pk.allowCredentials = pk.allowCredentials.map((c: any) => ({ ...c, id: b64uToBuf(c.id) }))
  }
  const cred = (await navigator.credentials.get({
    publicKey: pk as PublicKeyCredentialRequestOptions,
  })) as PublicKeyCredential | null
  if (!cred) throw new Error('Passkey sign-in was canceled')
  const res = cred.response as AuthenticatorAssertionResponse
  return {
    id: cred.id,
    rawId: bufToB64u(cred.rawId),
    type: cred.type,
    authenticatorAttachment: cred.authenticatorAttachment || undefined,
    clientExtensionResults: cred.getClientExtensionResults?.() ?? {},
    response: {
      clientDataJSON: bufToB64u(res.clientDataJSON),
      authenticatorData: bufToB64u(res.authenticatorData),
      signature: bufToB64u(res.signature),
      userHandle: res.userHandle ? bufToB64u(res.userHandle) : undefined,
    },
  }
}

/**
 * Turns a ceremony failure into something worth showing. The WebAuthn
 * errors are DOMExceptions whose message is empty or spec prose, so
 * the name is the only usable signal.
 */
export function ceremonyErrorMessage(e: unknown, fallback: string): string {
  const name = (e as { name?: string })?.name
  switch (name) {
    case 'NotAllowedError':
      return 'The passkey prompt was dismissed or timed out'
    case 'InvalidStateError':
      return 'This device already holds a passkey for this account'
    case 'SecurityError':
      return 'The page origin does not allow passkeys, check the URL is the one you normally use'
    case 'NotSupportedError':
      return 'This device cannot create the kind of passkey Gosint asks for'
    case 'AbortError':
      return 'The passkey prompt was canceled'
    default:
      return (e as { message?: string })?.message || fallback
  }
}
