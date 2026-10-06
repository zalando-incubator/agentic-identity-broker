export const storyViewports = {
  narrow320: { name: 'Narrow 320px', styles: { width: '320px', height: '800px' } },
  mobile: { name: 'Mobile', styles: { width: '375px', height: '812px' } },
  tablet: { name: 'Tablet', styles: { width: '768px', height: '1024px' } },
  desktop: { name: 'Desktop', styles: { width: '1280px', height: '720px' } },
} as const;

export type MatrixViewport = 'mobile' | 'tablet' | 'desktop';

export function viewportDimensions(name: keyof typeof storyViewports) {
  const { width, height } = storyViewports[name].styles;
  return { width: Number.parseInt(width, 10), height: Number.parseInt(height, 10) };
}
