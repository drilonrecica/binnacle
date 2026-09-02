import {
  applyPreferences,
  defaultPreferences,
  loadServerPreferences,
  preferences,
  resolveTheme,
  saveServerPreferences,
  type UserPreferences,
} from './preferences';

const sidebarKey = 'binnacle.sidebar';

/**
 * Reactive view of the user's preferences. `value` is server-authoritative
 * once `load()` resolves; before that it mirrors localStorage so the theme
 * applies without a flash.
 */
class PreferencesStore {
  value = $state<UserPreferences>(defaultPreferences);
  loaded = $state(false);
  saving = $state(false);
  /** Local-only UI state that should not round-trip to the server. */
  sidebarCollapsed = $state(false);

  readonly resolvedTheme = $derived(resolveTheme(this.value.theme));

  /** Applies the locally mirrored preferences before the session check. */
  bootstrap() {
    this.value = applyPreferences(preferences());
    try {
      this.sidebarCollapsed = localStorage.getItem(sidebarKey) === 'collapsed';
    } catch {
      this.sidebarCollapsed = false;
    }
    matchMedia('(prefers-color-scheme: dark)').addEventListener(
      'change',
      () => {
        if (this.value.theme === 'system') applyPreferences(this.value);
      },
    );
  }

  async load() {
    this.value = await loadServerPreferences();
    this.loaded = true;
    return this.value;
  }

  async save(patch: Partial<UserPreferences>) {
    this.saving = true;
    try {
      this.value = await saveServerPreferences({ ...this.value, ...patch });
      return this.value;
    } finally {
      this.saving = false;
    }
  }

  toggleSidebar(collapsed = !this.sidebarCollapsed) {
    this.sidebarCollapsed = collapsed;
    try {
      localStorage.setItem(sidebarKey, collapsed ? 'collapsed' : 'expanded');
    } catch {
      // Local persistence is a convenience only.
    }
  }
}

export const prefs = new PreferencesStore();
