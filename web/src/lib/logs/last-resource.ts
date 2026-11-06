/**
 * The resource whose detail page was viewed last, so the retired `/logs`
 * address (bookmarks, the "Logs" start page) opens that resource's Logs tab.
 * Browser-local and best-effort: without storage `/logs` asks for a resource.
 */
const key = 'binnacle.logs.resource';

export function rememberLogsResource(id: string): void {
  try {
    localStorage.setItem(key, id);
  } catch {
    /* Remembering is a convenience. */
  }
}

export function lastLogsResource(): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}
