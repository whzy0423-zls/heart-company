import { describe, expect, it } from 'vitest';
import { clampPosterPosition, dragPosterPosition } from './distribution-poster-layout';

describe('poster drag coordinates', () => {
  it('converts scaled desktop and mobile pointer distances to export pixels', () => {
    expect(dragPosterPosition(62, 1010, 50, -100, 360, 640, 176, 176)).toEqual({ x: 162, y: 810 });
    expect(dragPosterPosition(62, 1010, 25, -50, 180, 320, 176, 176)).toEqual({ x: 162, y: 810 });
  });
  it('keeps the complete QR and invitation inside the canvas', () => {
    expect(clampPosterPosition(-50, 2000, 220, 220)).toEqual({ x: 0, y: 1060 });
    expect(clampPosterPosition(1000, 2000, 350, 38)).toEqual({ x: 370, y: 1242 });
    expect(clampPosterPosition(0, 0, 176, 176)).toEqual({ x: 0, y: 0 });
  });
});
