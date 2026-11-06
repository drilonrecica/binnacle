import type { Component } from 'svelte';
import Gauge from '@lucide/svelte/icons/gauge';
import Boxes from '@lucide/svelte/icons/boxes';
import Server from '@lucide/svelte/icons/server';
import Siren from '@lucide/svelte/icons/siren';
import History from '@lucide/svelte/icons/history';
import Settings from '@lucide/svelte/icons/settings';
import { homePath, type RouteName } from '../router';

export interface NavItem {
  route: RouteName;
  href: string;
  label: string;
  icon: Component;
  /** Route names that highlight this item. */
  matches: RouteName[];
  keywords: string[];
}

/** Primary navigation in the order DESIGN §IA fixes. */
export const navigation: NavItem[] = [
  {
    route: 'headroom',
    href: homePath,
    label: 'Headroom',
    icon: Gauge,
    matches: ['headroom'],
    keywords: ['home', 'overview', 'dashboard', 'capacity', 'room left'],
  },
  {
    route: 'resources',
    href: '/resources',
    label: 'Resources',
    icon: Boxes,
    matches: ['resources', 'resource', 'logs'],
    keywords: ['services', 'containers', 'apps', 'logs'],
  },
  {
    route: 'server',
    href: '/server',
    label: 'Server',
    icon: Server,
    matches: ['server'],
    keywords: ['host', 'processes', 'filesystems', 'disks', 'collectors'],
  },
  {
    route: 'incidents',
    href: '/incidents',
    label: 'Incidents',
    icon: Siren,
    matches: ['incidents', 'incident'],
    keywords: ['alerts', 'firing', 'checks', 'silences'],
  },
  {
    route: 'changes',
    href: '/changes',
    label: 'Changes',
    icon: History,
    matches: ['changes'],
    keywords: ['activity', 'events', 'timeline', 'history', 'deploys'],
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

/** Pages in the phone tab bar; the rest live behind "More". */
export const mobilePrimary: RouteName[] = [
  'headroom',
  'resources',
  'server',
  'incidents',
  'changes',
];

/** Whether `item` is the current page; unknown routes highlight nothing. */
export function isActive(item: NavItem, route: RouteName | null): boolean {
  return route !== null && item.matches.includes(route);
}

export function pageTitle(route: RouteName | null): string {
  switch (route) {
    case 'resource':
      return 'Resource';
    case 'incident':
      return 'Incident';
    case 'logs':
      return 'Logs';
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
