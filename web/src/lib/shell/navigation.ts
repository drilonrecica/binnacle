import type { Component } from 'svelte';
import LayoutDashboard from '@lucide/svelte/icons/layout-dashboard';
import Boxes from '@lucide/svelte/icons/boxes';
import Server from '@lucide/svelte/icons/server';
import Siren from '@lucide/svelte/icons/siren';
import Activity from '@lucide/svelte/icons/activity';
import ScrollText from '@lucide/svelte/icons/scroll-text';
import Settings from '@lucide/svelte/icons/settings';
import type { RouteName } from '../router';

export interface NavItem {
  route: RouteName;
  href: string;
  label: string;
  icon: Component;
  /** Route names that highlight this item. */
  matches: RouteName[];
  keywords: string[];
}

export const navigation: NavItem[] = [
  {
    route: 'overview',
    href: '/overview',
    label: 'Overview',
    icon: LayoutDashboard,
    matches: ['overview'],
    keywords: ['home', 'dashboard', 'watch'],
  },
  {
    route: 'resources',
    href: '/resources',
    label: 'Resources',
    icon: Boxes,
    matches: ['resources', 'resource'],
    keywords: ['services', 'containers', 'apps'],
  },
  {
    route: 'host',
    href: '/host',
    label: 'Host',
    icon: Server,
    matches: ['host'],
    keywords: ['server', 'processes', 'filesystems', 'collectors'],
  },
  {
    route: 'alerts',
    href: '/alerts',
    label: 'Alerts',
    icon: Siren,
    matches: ['alerts', 'incident'],
    keywords: ['incidents', 'rules', 'checks', 'silences'],
  },
  {
    route: 'activity',
    href: '/activity',
    label: 'Activity',
    icon: Activity,
    matches: ['activity'],
    keywords: ['events', 'timeline', 'history'],
  },
  {
    route: 'logs',
    href: '/logs',
    label: 'Logs',
    icon: ScrollText,
    matches: ['logs'],
    keywords: ['container logs', 'output'],
  },
  {
    route: 'settings',
    href: '/settings',
    label: 'Settings',
    icon: Settings,
    matches: ['settings'],
    keywords: ['preferences', 'retention', 'notifications', 'access', 'tokens'],
  },
];

/** Items shown in the mobile tab bar; the rest live behind "More". */
export const mobilePrimary: RouteName[] = [
  'overview',
  'resources',
  'alerts',
  'activity',
];

export function pageTitle(route: RouteName | null): string {
  switch (route) {
    case 'resource':
      return 'Resource';
    case 'incident':
      return 'Incident';
    case 'login':
      return 'Sign in';
    case 'setup':
      return 'Set up';
    case 'onboarding':
      return 'Welcome';
    default:
      return (
        navigation.find((item) => item.route === route)?.label ?? 'Binnacle'
      );
  }
}
