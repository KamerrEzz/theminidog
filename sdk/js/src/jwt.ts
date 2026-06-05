import { createHmac } from 'node:crypto';

function base64urlEncode(str: string): string {
  return Buffer.from(str)
    .toString('base64')
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=/g, '');
}

export function signJWT(secret: string, payload: Record<string, unknown>): string {
  const header = base64urlEncode(JSON.stringify({ alg: 'HS256', typ: 'JWT' }));
  const body = base64urlEncode(JSON.stringify(payload));
  const data = `${header}.${body}`;
  const sig = createHmac('sha256', secret).update(data).digest('base64url');
  return `${data}.${sig}`;
}

export function mintAgentToken(secret: string): string {
  const now = Math.floor(Date.now() / 1000);
  return signJWT(secret, {
    iss: 'miniobserv-agent',
    iat: now,
    exp: now + 86400, // 24h
  });
}
