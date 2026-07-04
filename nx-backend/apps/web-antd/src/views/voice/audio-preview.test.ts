import { describe, expect, it } from 'vitest';

import { withVoicePreviewToken } from './audio-preview';

describe('voice audio preview helpers', () => {
  it('does not expose backend token in preview urls', () => {
    expect(withVoicePreviewToken('/api/upload-assets/13', 'abc 123')).toBe(
      '/api/upload-assets/13',
    );
    expect(withVoicePreviewToken('/api/upload-assets/13?download=1', 'abc')).toBe(
      '/api/upload-assets/13?download=1',
    );
  });

  it('leaves empty and public urls unchanged', () => {
    expect(withVoicePreviewToken('', 'abc')).toBe('');
    expect(withVoicePreviewToken('/api/upload-assets/13', '')).toBe(
      '/api/upload-assets/13',
    );
    expect(withVoicePreviewToken('https://cdn.example.com/a.mp3', 'abc')).toBe(
      'https://cdn.example.com/a.mp3',
    );
  });
});
