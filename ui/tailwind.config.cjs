// ui/tailwind.config.cjs
module.exports = {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: { brand: "#74ecbe" },
      fontFamily: {
        'mono': ['Monofur', 'ui-monospace', 'SFMono-Regular', 'Consolas', 'Liberation Mono', 'Menlo', 'monospace'],
        'sans': ['Monofur', 'ui-sans-serif', 'system-ui', 'sans-serif'],
        'brand': ['ui-sans-serif', 'system-ui', 'sans-serif']
      }
    }
  },
  plugins: []
};
