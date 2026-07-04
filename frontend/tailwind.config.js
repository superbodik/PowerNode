export default {
  content: ['./index.html', './src/**/*.{ts,tsx}'],
  theme: {
    extend: {
      colors: {
        bg: 'var(--bg)',
        surface: 'var(--surface)',
        border: 'var(--border)',
        pink: 'var(--pink)',
        'pink-b': 'var(--pink-b)',
        muted: 'var(--muted)',
        text: 'var(--text)',
      },
    },
  },
  plugins: [],
};
