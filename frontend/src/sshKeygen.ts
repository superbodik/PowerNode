export interface GeneratedKeyPair {
  publicKeyLine: string;
  privateKeyPem: string;
}

export function canGenerateKeyPair(): boolean {
  return typeof crypto !== 'undefined' && !!crypto.subtle && !!crypto.subtle.generateKey;
}

function toBase64(bytes: Uint8Array): string {
  let binary = '';
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary);
}

function pemArmor(buf: ArrayBuffer, label: string): string {
  const b64 = toBase64(new Uint8Array(buf));
  const lines = b64.match(/.{1,64}/g) ?? [b64];
  return `-----BEGIN ${label}-----\n${lines.join('\n')}\n-----END ${label}-----\n`;
}

function sshString(data: Uint8Array): Uint8Array {
  const out = new Uint8Array(4 + data.length);
  new DataView(out.buffer).setUint32(0, data.length, false);
  out.set(data, 4);
  return out;
}

function concatBytes(...parts: Uint8Array[]): Uint8Array {
  const total = parts.reduce((n, p) => n + p.length, 0);
  const out = new Uint8Array(total);
  let offset = 0;
  for (const p of parts) {
    out.set(p, offset);
    offset += p.length;
  }
  return out;
}

export async function generateEd25519KeyPair(comment: string): Promise<GeneratedKeyPair> {
  const keyPair = (await crypto.subtle.generateKey({ name: 'Ed25519' }, true, [
    'sign',
    'verify',
  ])) as CryptoKeyPair;

  const pkcs8 = await crypto.subtle.exportKey('pkcs8', keyPair.privateKey);
  const rawPublic = await crypto.subtle.exportKey('raw', keyPair.publicKey);

  const typeBytes = new TextEncoder().encode('ssh-ed25519');
  const blob = concatBytes(sshString(typeBytes), sshString(new Uint8Array(rawPublic)));
  const publicKeyLine = `ssh-ed25519 ${toBase64(blob)} ${comment}`;

  return { publicKeyLine, privateKeyPem: pemArmor(pkcs8, 'PRIVATE KEY') };
}

export function downloadPrivateKey(pem: string, filename: string) {
  const blob = new Blob([pem], { type: 'application/x-pem-file' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  URL.revokeObjectURL(url);
}
