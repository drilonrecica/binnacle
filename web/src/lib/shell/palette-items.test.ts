import { describe, expect, it } from 'vitest';
import { rankItems } from '../ui/palette';
import { destinationItems } from './palette-items';

const resources = [
  { id: 'res_api', name: 'api', status: 'healthy', project: 'shop' },
  { id: 'res_db', name: 'postgres', status: 'degraded' },
];

describe('destinationItems', () => {
  it('lists the six pages in navigation order', () => {
    expect(destinationItems([]).pages.map((item) => item.label)).toEqual([
      'Headroom',
      'Resources',
      'Server',
      'Incidents',
      'Changes',
      'Settings',
    ]);
  });

  it('offers "Logs for …" for every resource, found by typing logs', () => {
    const { pages, resources: items, logs } = destinationItems(resources);
    expect(logs.map((item) => [item.label, item.href])).toEqual([
      ['Logs for api', '/logs?resource=res_api'],
      ['Logs for postgres', '/logs?resource=res_db'],
    ]);
    const ranked = rankItems([...pages, ...items, ...logs], 'logs for post');
    expect(ranked[0]?.id).toBe('logs:res_db');
  });

  it('links resources by id but labels them by name', () => {
    const [api] = destinationItems(resources).resources;
    expect(api).toMatchObject({
      label: 'api',
      hint: 'shop',
      href: '/resources/res_api',
    });
  });
});
