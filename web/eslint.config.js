import js from '@eslint/js';
import svelte from 'eslint-plugin-svelte';
import globals from 'globals';
import tseslint from 'typescript-eslint';

const srcFiles = ['src/**/*.ts', 'src/**/*.svelte'];

export default [
  {
    ignores: [
      'dist/**',
      'coverage/**',
      'node_modules/**',
      'playwright-report/**',
      'test-results/**',
    ],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  ...svelte.configs.recommended,
  {
    languageOptions: {
      globals: globals.browser,
    },
  },
  {
    files: ['scripts/**/*.mjs'],
    languageOptions: {
      globals: globals.node,
    },
  },
  {
    files: ['**/*.svelte', '**/*.svelte.ts', '**/*.svelte.js'],
    languageOptions: {
      parserOptions: {
        extraFileExtensions: ['.svelte'],
        parser: tseslint.parser,
      },
    },
  },
  // Type-aware rules run on the application source, which tsconfig.json
  // includes. Tests, e2e specs and config files keep the syntactic rules.
  // The parser stays whatever the file type already uses (the Svelte parser
  // delegates its <script> blocks to the TypeScript one below).
  ...tseslint.configs.recommendedTypeChecked.map((config) => {
    // Configs that carry their own `files` (the TS-only overrides) keep it.
    const scoped = { ...config, files: config.files ?? srcFiles };
    delete scoped.languageOptions;
    return scoped;
  }),
  {
    files: ['src/**/*.ts'],
    languageOptions: {
      parser: tseslint.parser,
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
      },
    },
  },
  {
    files: ['src/**/*.svelte', 'src/**/*.svelte.ts'],
    languageOptions: {
      parserOptions: {
        projectService: true,
        tsconfigRootDir: import.meta.dirname,
        extraFileExtensions: ['.svelte'],
        parser: tseslint.parser,
      },
    },
  },
  {
    // typescript-eslint cannot see the types svelte-check derives for
    // components, props and `.svelte` imports, so its any-propagation rules
    // report false positives there. svelte-check (strict) covers that ground.
    files: ['src/**/*.svelte', 'src/**/*.test.ts'],
    rules: {
      '@typescript-eslint/no-unsafe-assignment': 'off',
      '@typescript-eslint/no-unsafe-member-access': 'off',
      '@typescript-eslint/no-unsafe-call': 'off',
      '@typescript-eslint/no-unsafe-return': 'off',
      '@typescript-eslint/no-unsafe-argument': 'off',
    },
  },
  {
    // Server-provided states are open unions: the known values document the
    // type and `| string` keeps unknown future ones from breaking the UI.
    files: ['src/**/*.ts', 'src/**/*.svelte'],
    rules: { '@typescript-eslint/no-redundant-type-constituents': 'off' },
  },
  {
    // Tests stub async APIs, hand around mock methods and eval the theme
    // script on purpose.
    files: ['src/**/*.test.ts', 'src/lib/ui/testing.ts'],
    rules: {
      '@typescript-eslint/require-await': 'off',
      '@typescript-eslint/unbound-method': 'off',
      '@typescript-eslint/no-base-to-string': 'off',
      '@typescript-eslint/no-unnecessary-type-assertion': 'off',
      '@typescript-eslint/no-implied-eval': 'off',
    },
  },
];
