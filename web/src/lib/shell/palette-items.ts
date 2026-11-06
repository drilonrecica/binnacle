import type { LiveSnapshot } from '../live.svelte';
import { logsPath, resourcePath } from '../router';
import type { PaletteItem } from '../ui/palette';
import { navigation } from './navigation';

type PaletteResource = Pick<
  LiveSnapshot['resources'][number],
  'id' | 'name' | 'status' | 'project' | 'environment' | 'context' | 'category'
>;

const settingsSections: Array<[string, string]> = [
  ['general', 'General settings'],
  ['data', 'Data & retention'],
  ['access', 'Access & sessions'],
  ['notifications', 'Notification channels'],
  ['alerting', 'Alert rules & checks'],
  ['appearance', 'Appearance'],
  ['integrations', 'Integrations & API'],
  ['system', 'System health & diagnostics'],
];

function hintFor(resource: PaletteResource): string {
  return (
    [resource.project, resource.environment].filter(Boolean).join(' / ') ||
    resource.context ||
    resource.category ||
    ''
  );
}

/**
 * Everything the command palette can jump to: the six pages, each resource,
 * "Logs for <resource>", and the settings sections. Actions are added by the
 * shell because they depend on preferences.
 */
export function destinationItems(resources: readonly PaletteResource[]): {
  pages: PaletteItem[];
  resources: PaletteItem[];
  logs: PaletteItem[];
  settings: PaletteItem[];
} {
  return {
    pages: navigation.map((item) => ({
      id: `page:${item.route}`,
      group: 'Pages',
      label: item.label,
      href: item.href,
      keywords: item.keywords,
    })),
    resources: resources.map((resource) => ({
      id: `resource:${resource.id}`,
      group: 'Resources',
      label: resource.name,
      hint: hintFor(resource),
      href: resourcePath(resource.id),
      keywords: [resource.id, resource.status],
    })),
    logs: resources.map((resource) => ({
      id: `logs:${resource.id}`,
      group: 'Logs',
      label: `Logs for ${resource.name}`,
      hint: hintFor(resource),
      href: logsPath(resource.id),
      keywords: ['logs', 'output'],
    })),
    settings: settingsSections.map(([section, label]) => ({
      id: `settings:${section}`,
      group: 'Settings',
      label,
      href: `/settings/${section}`,
      keywords: ['settings'],
    })),
  };
}
