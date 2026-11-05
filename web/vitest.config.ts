import { svelteTesting } from '@testing-library/svelte/vite';
import { defineConfig, mergeConfig } from 'vitest/config';

import viteConfig from './vite.config';

export default mergeConfig(
  viteConfig,
  defineConfig({
    // Resolves Svelte's browser build and cleans the DOM after each component
    // test. Component tests opt into jsdom with a `@vitest-environment jsdom`
    // docblock; everything else stays on the faster node environment.
    plugins: [svelteTesting()],
    test: {
      environment: 'node',
      include: ['src/**/*.test.ts', 'scripts/**/*.test.mjs'],
    },
  }),
);
