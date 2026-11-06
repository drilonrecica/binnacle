import { createHmac } from 'node:crypto';

const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';
const period = 30;

function base32Decode(encoded: string): Buffer {
  let bits = 0;
  let value = 0;
  const bytes: number[] = [];
  for (const char of encoded.toUpperCase().replace(/=+$/, '')) {
    const index = alphabet.indexOf(char);
    if (index < 0) throw new Error(`invalid base32 character ${char}`);
    value = (value << 5) | index;
    bits += 5;
    if (bits >= 8) {
      bytes.push((value >>> (bits - 8)) & 0xff);
      bits -= 8;
    }
  }
  return Buffer.from(bytes);
}

/**
 * An authenticator app for the acceptance flows: RFC 6238 written against the
 * standard, not imported from the product. It hands out codes for strictly
 * increasing steps, since Binnacle refuses a step it has already accepted,
 * and stays inside the server's one-step drift window.
 */
export class Authenticator {
  private readonly seed: Buffer;
  private last = 0;

  constructor(encodedSeed: string) {
    this.seed = base32Decode(encodedSeed);
  }

  codeAt(step: number): string {
    const message = Buffer.alloc(8);
    message.writeBigUInt64BE(BigInt(step));
    const sum = createHmac('sha1', this.seed).update(message).digest();
    const offset = sum[sum.length - 1]! & 0x0f;
    const value = sum.readUInt32BE(offset) & 0x7fffffff;
    return String(value % 1_000_000).padStart(6, '0');
  }

  /**
   * The earliest acceptable step later than every code handed out so far.
   * The previous step drops out of the window at the next boundary, so it is
   * only used with a few seconds to spare.
   */
  async next(): Promise<string> {
    for (;;) {
      const now = Date.now() / 1000;
      const current = Math.floor(now / period);
      const remainingMs = ((current + 1) * period - now) * 1000;
      const step = Math.max(this.last + 1, current - 1);
      if (step > current + 1 || (step === current - 1 && remainingMs < 5000)) {
        await new Promise((done) => setTimeout(done, remainingMs + 200));
        continue;
      }
      this.last = step;
      return this.codeAt(step);
    }
  }
}
