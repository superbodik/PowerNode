import type { Server } from '../types';

// Single source of truth for the OBS Server/Stream Key convention: Server is
// the bare RTMP endpoint, Stream Key is the relay secret. Keeping this in one
// place matters because getting these two swapped once already broke every
// stream in production (OBS concatenates Server-path + Stream Key into the
// published path, so a secret living in the wrong field silently mismatches
// what the relay's mediamtx config expects).
export function relaySecretOf(server: Server | null | undefined): string | undefined {
  return server?.environment?.RELAY_SECRET;
}

export function obsServerUrlFor(server: Server | null | undefined): string | null {
  if (!server?.primary_address || !relaySecretOf(server)) return null;
  const host = server.primary_address.split(':')[0];
  const port = server.environment?.RTMP_PORT || '1935';
  return `rtmp://${host}:${port}`;
}
