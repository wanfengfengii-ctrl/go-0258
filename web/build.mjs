// Deterministic build: copy the static console source into ../webembed/dist
// so the Go binary can embed it. No third-party dependencies are used.
import { cpSync, mkdirSync, rmSync } from 'node:fs';

const src = new URL('./src/', import.meta.url);
const dst = new URL('../webembed/dist/', import.meta.url);

rmSync(dst, { recursive: true, force: true });
mkdirSync(dst, { recursive: true });
cpSync(src, dst, { recursive: true });
console.log('built webembed/dist');
