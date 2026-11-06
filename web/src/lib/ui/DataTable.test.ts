// @vitest-environment jsdom
import { afterEach, describe, expect, it } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import DataTable, { type Column } from './DataTable.svelte';
import { viewport } from './media.svelte';

interface Row {
  id: string;
  name: string;
  cpu: number;
}

const rows: Row[] = [
  { id: 'a', name: 'api', cpu: 12 },
  { id: 'b', name: 'worker', cpu: 48 },
  { id: 'c', name: 'db', cpu: 3 },
];
const columns: Column<Row>[] = [
  { key: 'name', label: 'Name', sortable: true },
  { key: 'cpu', label: 'CPU', sortable: true, align: 'right' },
];

function props(extra: Record<string, unknown> = {}) {
  return {
    rows,
    columns,
    rowKey: (row: Row) => row.id,
    caption: 'Resources',
    ...extra,
  };
}

describe('DataTable', () => {
  afterEach(() => {
    viewport.width = 1280;
  });

  it('sorts by a column and reports it with aria-sort', async () => {
    render(DataTable<Row>, props());
    const cpu = screen.getByRole('columnheader', { name: 'CPU' });
    expect(cpu.getAttribute('aria-sort')).toBe('none');
    await fireEvent.click(within(cpu).getByRole('button'));
    expect(cpu.getAttribute('aria-sort')).toBe('descending');
    const names = screen
      .getAllByRole('row')
      .slice(1)
      .map((row) => row.querySelector('td')?.textContent?.trim());
    expect(names).toEqual(['worker', 'api', 'db']);
    await fireEvent.click(within(cpu).getByRole('button'));
    expect(cpu.getAttribute('aria-sort')).toBe('ascending');
  });

  it('builds a card form from the columns when no card snippet is given', () => {
    render(DataTable<Row>, props({ layout: 'cards' }));
    expect(screen.queryByRole('table')).toBeNull();
    const list = screen.getByRole('list', { name: 'Resources' });
    const cards = within(list).getAllByRole('listitem') as [
      HTMLElement,
      ...HTMLElement[],
    ];
    expect(cards).toHaveLength(3);
    // The first column heads the card; the rest are labelled fields.
    expect(cards[0].querySelector('.card-title')?.textContent?.trim()).toBe(
      'api',
    );
    expect(within(cards[0]).getByText('CPU').tagName).toBe('DT');
    expect(within(cards[0]).getByText('12').tagName).toBe('DD');
  });

  it('switches every table to cards below 900 px', () => {
    viewport.width = 899;
    render(DataTable<Row>, props());
    expect(screen.queryByRole('table')).toBeNull();
    expect(screen.getByRole('list', { name: 'Resources' })).toBeTruthy();
  });

  it('keeps the table at 900 px and hides lg columns below 1200', () => {
    viewport.width = 900;
    render(
      DataTable<Row>,
      props({
        columns: [columns[0], { ...columns[1], hideBelow: 'lg' as const }],
      }),
    );
    expect(screen.getByRole('table')).toBeTruthy();
    expect(screen.queryByRole('columnheader', { name: 'CPU' })).toBeNull();
  });

  it('lets cards sort without column headers', async () => {
    render(DataTable<Row>, props({ layout: 'cards' }));
    const select = screen.getByLabelText('Sort') as HTMLSelectElement;
    select.value = 'cpu:asc';
    await fireEvent.change(select);
    const titles = [...document.querySelectorAll('.card-title')].map((node) =>
      node.textContent?.trim(),
    );
    expect(titles).toEqual(['db', 'api', 'worker']);
  });
});
