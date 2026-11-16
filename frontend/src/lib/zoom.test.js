import { describe, it, expect } from 'vitest';
import { zoomForWidth } from './zoom.js';

describe('zoomForWidth (zoom auto selon l\'écran)', () => {
  it('écran Mac large → 100 %, jamais plus', () => {
    expect(zoomForWidth(2000)).toBe(1);
    expect(zoomForWidth(2560)).toBe(1);
  });
  it('PC 150 % (~1707 px) → 85 %', () => {
    expect(zoomForWidth(1707)).toBe(0.85);
  });
  it('petit écran → plancher 70 %', () => {
    expect(zoomForWidth(1100)).toBe(0.7);
  });
  it('largeur inconnue → 100 %', () => {
    expect(zoomForWidth(0)).toBe(1);
  });
});