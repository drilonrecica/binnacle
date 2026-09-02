import {
  autoUpdate,
  computePosition,
  flip,
  offset,
  shift,
  size,
  type Placement,
} from '@floating-ui/dom';

export interface FloatingOptions {
  placement?: Placement;
  gap?: number;
  /** Match the reference width (used by comboboxes). */
  matchWidth?: boolean;
}

/**
 * Positions `floating` relative to `reference` and keeps it updated while
 * either moves. Returns a cleanup function.
 */
export function attachFloating(
  reference: HTMLElement,
  floating: HTMLElement,
  options: FloatingOptions = {},
): () => void {
  const { placement = 'bottom-start', gap = 6, matchWidth = false } = options;
  floating.style.position = 'fixed';
  floating.style.top = '0';
  floating.style.left = '0';
  const update = async () => {
    const middleware = [offset(gap), flip(), shift({ padding: 8 })];
    if (matchWidth) {
      middleware.push(
        size({
          apply({ rects, elements }) {
            elements.floating.style.minWidth = `${rects.reference.width}px`;
          },
        }),
      );
    }
    const position = await computePosition(reference, floating, {
      placement,
      strategy: 'fixed',
      middleware,
    });
    floating.style.transform = `translate(${Math.round(position.x)}px, ${Math.round(position.y)}px)`;
    floating.dataset.placement = position.placement;
  };
  return autoUpdate(reference, floating, () => void update());
}
