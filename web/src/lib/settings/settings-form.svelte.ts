import { untrack } from 'svelte';
import type { SettingsSnapshot } from '../api/settings';
import { errorMessage, isApiError } from '../api/client';
import { leaveGuard } from '../leave-guard.svelte';
import { toasts } from '../ui/toast.svelte';

/**
 * Tracks edits to the setting keys of one card with dirty state, validation,
 * and a single save that sends only the changed keys against the current
 * revision. One form backs one card (UI-8). Created during component setup,
 * it registers with the leave guard for that component's lifetime (UI-21).
 */
export class SettingsForm {
  values = $state<Record<string, string>>({});
  errors = $state<Record<string, string>>({});
  saving = $state(false);
  error = $state('');
  /** False until the first snapshot has filled the values. */
  loaded = $state(false);

  private keys: string[];
  private validators: Record<string, (value: string) => string | null>;
  private snapshot: () => SettingsSnapshot | null;
  private apply: (changes: Record<string, string>) => Promise<SettingsSnapshot>;

  constructor(options: {
    keys: string[];
    snapshot: () => SettingsSnapshot | null;
    apply: (changes: Record<string, string>) => Promise<SettingsSnapshot>;
    validators?: Record<string, (value: string) => string | null>;
  }) {
    this.keys = options.keys;
    this.snapshot = options.snapshot;
    this.values = Object.fromEntries(options.keys.map((key) => [key, '']));
    this.apply = options.apply;
    this.validators = options.validators ?? {};
    $effect(() => untrack(() => leaveGuard.register(() => this.dirty)));
  }

  /**
   * Follows a new snapshot without losing edits: fills the values on first
   * load and whenever this card has nothing unsaved. A save in another card
   * replaces the snapshot and must not wipe this card's edits.
   */
  sync() {
    untrack(() => {
      if (!this.loaded || !this.dirty) this.reset();
    });
  }

  /** Reloads the editable values from the snapshot, discarding edits. */
  reset() {
    const snapshot = this.snapshot();
    const next: Record<string, string> = {};
    for (const key of this.keys) next[key] = snapshot?.values[key]?.value ?? '';
    this.values = next;
    this.errors = {};
    this.error = '';
    if (snapshot) this.loaded = true;
  }

  get dirty(): boolean {
    const snapshot = this.snapshot();
    if (!this.loaded || !snapshot) return false;
    return this.keys.some(
      (key) =>
        (snapshot?.values[key]?.value ?? '') !== (this.values[key] ?? ''),
    );
  }

  changes(): Record<string, string> {
    const snapshot = this.snapshot();
    const out: Record<string, string> = {};
    for (const key of this.keys) {
      const value = this.values[key] ?? '';
      if ((snapshot?.values[key]?.value ?? '') !== value) out[key] = value;
    }
    return out;
  }

  validate(): boolean {
    const errors: Record<string, string> = {};
    for (const key of this.keys) {
      const validator = this.validators[key];
      if (!validator) continue;
      const problem = validator(this.values[key] ?? '');
      if (problem) errors[key] = problem;
    }
    this.errors = errors;
    return Object.keys(errors).length === 0;
  }

  async save(label = 'Settings saved') {
    if (!this.dirty || this.saving) return;
    if (!this.validate()) return;
    this.saving = true;
    this.error = '';
    try {
      await this.apply(this.changes());
      this.reset();
      toasts.success(label);
    } catch (reason) {
      if (isApiError(reason, 'settings_conflict')) {
        this.error =
          'Settings changed elsewhere. Reload the page and try again.';
      } else {
        this.error = errorMessage(reason);
      }
    } finally {
      this.saving = false;
    }
  }
}
